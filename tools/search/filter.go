package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ganigeorgiev/fexpr"
	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/tools/security"
	"github.com/arief-fajri/pgbase/tools/store"
	"github.com/spf13/cast"
)

// FilterData is a filter expression string following the `fexpr` package grammar.
//
// The filter string can also contain dbx placeholder parameters (eg. "title = {:name}"),
// that will be safely replaced and properly quoted inplace with the placeholderReplacements values.
//
// Example:
//
//	var filter FilterData = "id = null || (name = 'test' && status = true) || (total >= {:min} && total <= {:max})"
//	resolver := search.NewSimpleFieldResolver("id", "name", "status")
//	expr, err := filter.BuildExpr(resolver, dbx.Params{"min": 100, "max": 200})
type FilterData string

// parsedFilterData holds a cache with previously parsed filter data expressions
// (initialized with some preallocated empty data map)
var parsedFilterData = store.New(make(map[string][]fexpr.ExprGroup, 50))

// BuildExpr parses the current filter data and returns a new db WHERE expression.
//
// The filter string can also contain dbx placeholder parameters (eg. "title = {:name}"),
// that will be safely replaced and properly quoted inplace with the placeholderReplacements values.
//
// The parsed expressions are limited up to DefaultFilterExprLimit.
// Use [FilterData.BuildExprWithLimit] if you want to set a custom limit.
func (f FilterData) BuildExpr(
	fieldResolver FieldResolver,
	placeholderReplacements ...dbx.Params,
) (dbx.Expression, error) {
	return f.BuildExprWithLimit(fieldResolver, DefaultFilterExprLimit, placeholderReplacements...)
}

// BuildExpr parses the current filter data and returns a new db WHERE expression.
//
// The filter string can also contain dbx placeholder parameters (eg. "title = {:name}"),
// that will be safely replaced and properly quoted inplace with the placeholderReplacements values.
func (f FilterData) BuildExprWithLimit(
	fieldResolver FieldResolver,
	maxExpressions int,
	placeholderReplacements ...dbx.Params,
) (dbx.Expression, error) {
	raw := string(f)

	// replace the placeholder params in the raw string filter
	for _, p := range placeholderReplacements {
		for key, value := range p {
			var replacement string
			switch v := value.(type) {
			case nil:
				replacement = "null"
			case bool, float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
				replacement = cast.ToString(v)
			default:
				replacement = cast.ToString(v)

				// try to json serialize as fallback
				if replacement == "" {
					raw, _ := json.Marshal(v)
					replacement = string(raw)
				}

				replacement = strconv.Quote(replacement)
			}
			raw = strings.ReplaceAll(raw, "{:"+key+"}", replacement)
		}
	}

	cacheKey := raw + "/" + strconv.Itoa(maxExpressions)

	if data, ok := parsedFilterData.GetOk(cacheKey); ok {
		return buildParsedFilterExpr(data, fieldResolver, &maxExpressions)
	}

	data, err := fexpr.Parse(raw)
	if err != nil {
		// depending on the users demand we may allow empty expressions
		// (aka. expressions consisting only of whitespaces or comments)
		// but for now disallow them as it seems unnecessary
		// if errors.Is(err, fexpr.ErrEmpty) {
		// return dbx.NewExp("1=1"), nil
		// }

		return nil, err
	}

	// store in cache
	// (the limit size is arbitrary and it is there to prevent the cache growing too big)
	parsedFilterData.SetIfLessThanLimit(cacheKey, data, 500)

	return buildParsedFilterExpr(data, fieldResolver, &maxExpressions)
}

func buildParsedFilterExpr(data []fexpr.ExprGroup, fieldResolver FieldResolver, maxExpressions *int) (dbx.Expression, error) {
	if len(data) == 0 {
		return nil, fexpr.ErrEmpty
	}

	result := &concatExpr{separator: " "}

	for _, group := range data {
		var expr dbx.Expression
		var exprErr error

		switch item := group.Item.(type) {
		case fexpr.Expr:
			if *maxExpressions <= 0 {
				return nil, ErrFilterExprLimit
			}

			*maxExpressions--

			expr, exprErr = resolveTokenizedExpr(item, fieldResolver)
		case fexpr.ExprGroup:
			expr, exprErr = buildParsedFilterExpr([]fexpr.ExprGroup{item}, fieldResolver, maxExpressions)
		case []fexpr.ExprGroup:
			expr, exprErr = buildParsedFilterExpr(item, fieldResolver, maxExpressions)
		default:
			exprErr = errors.New("unsupported expression item")
		}

		if exprErr != nil {
			return nil, exprErr
		}

		if len(result.parts) > 0 {
			var op string
			if group.Join == fexpr.JoinOr {
				op = "OR"
			} else {
				op = "AND"
			}
			result.parts = append(result.parts, &opExpr{op})
		}

		result.parts = append(result.parts, expr)
	}

	return result, nil
}

func resolveTokenizedExpr(expr fexpr.Expr, fieldResolver FieldResolver) (dbx.Expression, error) {
	lResult, lErr := resolveToken(expr.Left, fieldResolver)
	if lErr != nil || lResult.Identifier == "" {
		return nil, fmt.Errorf("invalid left operand %q - %v", expr.Left.Literal, lErr)
	}

	rResult, rErr := resolveToken(expr.Right, fieldResolver)
	if rErr != nil || rResult.Identifier == "" {
		return nil, fmt.Errorf("invalid right operand %q - %v", expr.Right.Literal, rErr)
	}

	return buildResolversExpr(lResult, expr.Op, rResult)
}

func isPureParamPlaceholder(id string) bool {
	return strings.HasPrefix(id, "{:") && strings.HasSuffix(id, "}")
}

func buildResolversExpr(
	left *ResolverResult,
	op fexpr.SignOp,
	right *ResolverResult,
) (dbx.Expression, error) {
	// constant folding: when both operands are pure literal params
	// (no column identifiers), evaluate at compile time to produce
	// "1 = 1" / "1 = 0" instead of placeholder params.
	// This avoids PostgreSQL type inference failures (pgx cannot
	// encode float64 params without a column type anchor).
	if isPureParamPlaceholder(left.Identifier) &&
		isPureParamPlaceholder(right.Identifier) &&
		len(left.Params) == 1 && len(right.Params) == 1 {
		leftVal := firstParamValue(left.Params)
		rightVal := firstParamValue(right.Params)

		var result bool
		if ln, lErr := cast.ToFloat64E(leftVal); lErr == nil {
			if rn, rErr := cast.ToFloat64E(rightVal); rErr == nil {
				// both numeric - compare as numbers
				switch op {
				case fexpr.SignEq, fexpr.SignAnyEq:
					result = ln == rn
				case fexpr.SignNeq, fexpr.SignAnyNeq:
					result = ln != rn
				case fexpr.SignLt, fexpr.SignAnyLt:
					result = ln < rn
				case fexpr.SignLte, fexpr.SignAnyLte:
					result = ln <= rn
				case fexpr.SignGt, fexpr.SignAnyGt:
					result = ln > rn
				case fexpr.SignGte, fexpr.SignAnyGte:
					result = ln >= rn
				}

				if result {
					return dbx.NewExp("1 = 1"), nil
				}
				return dbx.NewExp("1 = 0"), nil
			}
		}

		// One or both operands are non-numeric (e.g. @request.auth.id IDs, text
		// fields, or mixed number/string literals). Compare as text strings at
		// build time: this keeps the SQL deterministic and avoids PostgreSQL
		// "operator does not exist: double precision = text" errors, while also
		// fixing cases where casting IDs/text to float64 (0) produced wrong
		// results (e.g. `id != ''`).
		lStr := cast.ToString(leftVal)
		rStr := cast.ToString(rightVal)
		switch op {
		case fexpr.SignEq, fexpr.SignAnyEq:
			result = lStr == rStr
		case fexpr.SignNeq, fexpr.SignAnyNeq:
			result = lStr != rStr
		case fexpr.SignLt, fexpr.SignAnyLt:
			result = lStr < rStr
		case fexpr.SignLte, fexpr.SignAnyLte:
			result = lStr <= rStr
		case fexpr.SignGt, fexpr.SignAnyGt:
			result = lStr > rStr
		case fexpr.SignGte, fexpr.SignAnyGte:
			result = lStr >= rStr
		}

		if result {
			return dbx.NewExp("1 = 1"), nil
		}
		return dbx.NewExp("1 = 0"), nil
	}

	var expr dbx.Expression

	switch op {
	case fexpr.SignEq, fexpr.SignAnyEq:
		expr = resolveEqualExpr(true, left, right)
	case fexpr.SignNeq, fexpr.SignAnyNeq:
		expr = resolveEqualExpr(false, left, right)
	case fexpr.SignLike, fexpr.SignAnyLike:
		// note: PostgreSQL LIKE is case-sensitive while SQLite LIKE is case-insensitive.
		// We use LOWER() on both sides to preserve backward-compatible case-insensitive matching.
		if len(right.Params) == 0 {
			expr = dbx.NewExp(fmt.Sprintf("LOWER(%s) LIKE LOWER(('%%' || %s || '%%')) ESCAPE '\\'", left.Identifier, right.Identifier), left.Params)
		} else {
			expr = dbx.NewExp(fmt.Sprintf("LOWER(%s) LIKE LOWER(%s) ESCAPE '\\'", left.Identifier, right.Identifier), mergeParams(left.Params, wrapLikeParams(right.Params)))
		}
	case fexpr.SignNlike, fexpr.SignAnyNlike:
		// note: PostgreSQL LIKE is case-sensitive while SQLite LIKE is case-insensitive.
		// We use LOWER() on both sides to preserve backward-compatible case-insensitive matching.
		if len(right.Params) == 0 {
			expr = dbx.NewExp(fmt.Sprintf("LOWER(%s) NOT LIKE LOWER(('%%' || %s || '%%')) ESCAPE '\\'", left.Identifier, right.Identifier), left.Params)
		} else {
			expr = dbx.NewExp(fmt.Sprintf("LOWER(%s) NOT LIKE LOWER(%s) ESCAPE '\\'", left.Identifier, right.Identifier), mergeParams(left.Params, wrapLikeParams(right.Params)))
		}
	case fexpr.SignLt, fexpr.SignAnyLt:
		li, ri := numericCompareIdentifiers(left, right)
		expr = dbx.NewExp(fmt.Sprintf("%s < %s", li, ri), mergeParams(left.Params, right.Params))
	case fexpr.SignLte, fexpr.SignAnyLte:
		li, ri := numericCompareIdentifiers(left, right)
		expr = dbx.NewExp(fmt.Sprintf("%s <= %s", li, ri), mergeParams(left.Params, right.Params))
	case fexpr.SignGt, fexpr.SignAnyGt:
		li, ri := numericCompareIdentifiers(left, right)
		expr = dbx.NewExp(fmt.Sprintf("%s > %s", li, ri), mergeParams(left.Params, right.Params))
	case fexpr.SignGte, fexpr.SignAnyGte:
		li, ri := numericCompareIdentifiers(left, right)
		expr = dbx.NewExp(fmt.Sprintf("%s >= %s", li, ri), mergeParams(left.Params, right.Params))
	}

	if expr == nil {
		return nil, fmt.Errorf("unknown expression operator %q", op)
	}

	// multi-match expressions
	if !isAnyMatchOp(op) {
		if left.MultiMatchSubQuery != nil && right.MultiMatchSubQuery != nil {
			mm := &manyVsManyExpr{
				left:  left,
				right: right,
				op:    op,
			}

			expr = dbx.Enclose(dbx.And(expr, mm))
		} else if left.MultiMatchSubQuery != nil {
			mm := &manyVsOneExpr{
				nullFallback: left.NullFallback,
				subQuery:     left.MultiMatchSubQuery,
				op:           op,
				otherOperand: right,
			}

			expr = dbx.Enclose(dbx.And(expr, mm))
		} else if right.MultiMatchSubQuery != nil {
			mm := &manyVsOneExpr{
				nullFallback: right.NullFallback,
				subQuery:     right.MultiMatchSubQuery,
				op:           op,
				otherOperand: left,
				inverse:      true,
			}

			expr = dbx.Enclose(dbx.And(expr, mm))
		}
	}

	if left.AfterBuild != nil {
		expr = left.AfterBuild(expr)
	}

	if right.AfterBuild != nil {
		expr = right.AfterBuild(expr)
	}

	return expr, nil
}

var normalizedIdentifiers = map[string]string{
	// if `null` field is missing, treat `null` identifier as NULL token
	"null": "NULL",
	// if `true` field is missing, treat `true` identifier as TRUE token
	//
	// NB! PostgreSQL requires actual boolean operands instead of the
	// integer 1/0 literals that SQLite tolerated, so we keep the
	// native TRUE/FALSE keywords (both backends understand them).
	"true": "TRUE",
	// if `false` field is missing, treat `false` identifier as FALSE token
	"false": "FALSE",
}

func resolveToken(token fexpr.Token, fieldResolver FieldResolver) (*ResolverResult, error) {
	switch token.Type {
	case fexpr.TokenIdentifier:
		// check for macros
		// ---
		if macroFunc, ok := identifierMacros[token.Literal]; ok {
			placeholder := "t" + security.PseudorandomString(8)

			macroValue, err := macroFunc()
			if err != nil {
				return nil, err
			}

			return &ResolverResult{
				Identifier: "{:" + placeholder + "}",
				Params:     dbx.Params{placeholder: macroValue},
			}, nil
		}

		// custom resolver
		// ---
		result, err := fieldResolver.Resolve(token.Literal)
		if err != nil || result.Identifier == "" {
			for k, v := range normalizedIdentifiers {
				if strings.EqualFold(k, token.Literal) {
					return &ResolverResult{Identifier: v}, nil
				}
			}
			return nil, err
		}

		return result, err
	case fexpr.TokenText:
		placeholder := "t" + security.PseudorandomString(8)

		return &ResolverResult{
			Identifier: "{:" + placeholder + "}",
			Params:     dbx.Params{placeholder: token.Literal},
		}, nil
	case fexpr.TokenNumber:
		placeholder := "t" + security.PseudorandomString(8)

		return &ResolverResult{
			Identifier: "{:" + placeholder + "}",
			Params:     dbx.Params{placeholder: cast.ToFloat64(token.Literal)},
		}, nil
	case fexpr.TokenFunction:
		fn, ok := TokenFunctions[token.Literal]
		if !ok {
			return nil, fmt.Errorf("unknown function %q", token.Literal)
		}

		args, _ := token.Meta.([]fexpr.Token)
		return fn(func(argToken fexpr.Token) (*ResolverResult, error) {
			return resolveToken(argToken, fieldResolver)
		}, args...)
	}

	return nil, fmt.Errorf("unsupported token type %q", token.Type)
}

// Resolves = and != expressions in an attempt to minimize the COALESCE
// usage and to gracefully handle null vs empty string normalizations.
//
// The expression `a = "" OR a is null` tends to perform better than
// `COALESCE(a, "") = ""` since the direct match can be accomplished
// with a seek while the COALESCE will induce a table scan.
func resolveEqualExpr(equal bool, left, right *ResolverResult) dbx.Expression {
	equalOp := "="
	nullEqualOp := "IS"
	concatOp := "OR"
	nullExpr := "IS NULL"
	if !equal {
		// PostgreSQL does not allow `IS NOT` with non-NULL operands.
		// Using `!=` is the standard non-equal operator.
		equalOp = "!="
		nullEqualOp = "IS DISTINCT FROM"
		concatOp = "AND"
		nullExpr = "IS NOT NULL"
	}

	// no coalesce fallback (eg. compare to a json field)
	// a IS b / a IS NOT b
	if left.NullFallback == NullFallbackDisabled ||
		right.NullFallback == NullFallbackDisabled {
		// PostgreSQL allows "IS"/"IS DISTINCT FROM" only when at least one of the
		// operands is a NULL literal. For two non-NULL operands (eg. a JSON field
		// compared to a bound value placeholder) we must use the regular
		// "="/"!=" operators, otherwise pgx fails with
		// "argument of IS must be type boolean, not type text/numeric".
		if left.Identifier != "NULL" && right.Identifier != "NULL" {
			return dbx.NewExp(
				fmt.Sprintf("%s %s %s", left.Identifier, equalOp, right.Identifier),
				mergeParams(left.Params, right.Params),
			)
		}
		return dbx.NewExp(
			fmt.Sprintf("%s %s %s", left.Identifier, nullEqualOp, right.Identifier),
			mergeParams(left.Params, right.Params),
		)
	}

	isLeftEmpty := isEmptyIdentifier(left) ||
		(left.NullFallback == NullFallbackAuto && len(left.Params) == 1 && hasEmptyParamValue(left))

	isRightEmpty := isEmptyIdentifier(right) ||
		(right.NullFallback == NullFallbackAuto && len(right.Params) == 1 && hasEmptyParamValue(right))

	// both operands are empty
	if isLeftEmpty && isRightEmpty {
		return dbx.NewExp(fmt.Sprintf("'' %s ''", emptyCompareOp(equalOp)), mergeParams(left.Params, right.Params))
	}

	// direct compare since at least one of the operands is known to be non-empty
	// eg. a = 'example'
	if isKnownNonEmptyIdentifier(left) || isKnownNonEmptyIdentifier(right) {
		leftIdentifier := left.Identifier
		if isLeftEmpty {
			leftIdentifier = "''"
		}
		rightIdentifier := right.Identifier
		if isRightEmpty {
			rightIdentifier = "''"
		}
		return dbx.NewExp(
			fmt.Sprintf("%s %s %s", leftIdentifier, equalOp, rightIdentifier),
			mergeParams(left.Params, right.Params),
		)
	}

	// "" = b OR b IS NULL
	// "" != b AND b IS NOT NULL
	if isLeftEmpty {
		return dbx.NewExp(
			fmt.Sprintf("('' %s %s %s %s %s)", emptyCompareOp(equalOp), right.Identifier, concatOp, right.Identifier, nullExpr),
			mergeParams(left.Params, right.Params),
		)
	}

	// a = "" OR a IS NULL
	// a != "" AND a IS NOT NULL
	if isRightEmpty {
		return dbx.NewExp(
			fmt.Sprintf("(%s %s '' %s %s %s)", left.Identifier, emptyCompareOp(equalOp), concatOp, left.Identifier, nullExpr),
			mergeParams(left.Params, right.Params),
		)
	}

	// fallback to a COALESCE comparison
	return dbx.NewExp(
		fmt.Sprintf(
			"COALESCE(%s, '') %s COALESCE(%s, '')",
			left.Identifier,
			equalOp,
			right.Identifier,
		),
		mergeParams(left.Params, right.Params),
	)
}

func hasEmptyParamValue(result *ResolverResult) bool {
	for _, p := range result.Params {
		switch v := p.(type) {
		case nil:
			return true
		case string:
			if v == "" {
				return true
			}
		}
	}

	return false
}

func isKnownNonEmptyIdentifier(result *ResolverResult) bool {
	if result.NullFallback == NullFallbackEnforced {
		return false
	}

	switch strings.ToLower(result.Identifier) {
	case "1", "0", "false", `true`:
		return true
	}

	return len(result.Params) > 0 && !hasEmptyParamValue(result) && !isEmptyIdentifier(result)
}

func isEmptyIdentifier(result *ResolverResult) bool {
	switch strings.ToLower(result.Identifier) {
	case "", "null", "''", `""`, "``":
		return true
	default:
		return false
	}
}

func isAnyMatchOp(op fexpr.SignOp) bool {
	switch op {
	case
		fexpr.SignAnyEq,
		fexpr.SignAnyNeq,
		fexpr.SignAnyLike,
		fexpr.SignAnyNlike,
		fexpr.SignAnyLt,
		fexpr.SignAnyLte,
		fexpr.SignAnyGt,
		fexpr.SignAnyGte:
		return true
	}

	return false
}

// firstParamValue returns the first non-zero param value.
func firstParamValue(params dbx.Params) any {
	for _, v := range params {
		return v
	}
	return nil
}

// isNumericParam reports whether the resolved operand is a single numeric
// literal param (produced from an fexpr number token).
func isNumericParam(result *ResolverResult) bool {
	if len(result.Params) != 1 {
		return false
	}
	switch firstParamValue(result.Params).(type) {
	case float64, float32, int, int64, int32, int16, int8, uint, uint64, uint32, uint16, uint8:
		return true
	}
	return false
}

// numericCompareIdentifiers returns the left/right SQL identifiers to use for a
// numeric comparison (<, <=, >, >=), casting a JSON-extracted text operand to
// numeric when it is compared against a numeric literal.
//
// PostgreSQL has no implicit text<->number comparison and pgx cannot encode a
// numeric literal into a text-typed placeholder (the JSON `#>>` extraction
// yields text). Casting the JSON side to numeric makes the comparison both
// valid and numerically correct, eg. `data.status > 200` (unlike SQLite, which
// coerces via its dynamic typing).
func numericCompareIdentifiers(left, right *ResolverResult) (string, string) {
	li, ri := left.Identifier, right.Identifier
	if left.JSONText && isNumericParam(right) {
		li = "(" + li + ")::numeric"
	}
	if right.JSONText && isNumericParam(left) {
		ri = "(" + ri + ")::numeric"
	}
	return li, ri
}

// mergeParams returns new dbx.Params where each provided params item
// is merged in the order they are specified.
func mergeParams(params ...dbx.Params) dbx.Params {
	result := dbx.Params{}

	for _, p := range params {
		for k, v := range p {
			result[k] = v
		}
	}

	return result
}

// @todo consider adding support for custom single character wildcard
//
// wrapLikeParams wraps each provided param value string with `%`
// if the param doesn't contain an explicit wildcard (`%`) character already.
func wrapLikeParams(params dbx.Params) dbx.Params {
	result := dbx.Params{}

	for k, v := range params {
		vStr := cast.ToString(v)
		if !containsUnescapedChar(vStr, '%') {
			// note: this is done to minimize the breaking changes and to preserve the original autoescape behavior
			vStr = escapeUnescapedChars(vStr, '\\', '%', '_')
			vStr = "%" + vStr + "%"
		}
		result[k] = vStr
	}

	return result
}

func escapeUnescapedChars(str string, escapeChars ...rune) string {
	rs := []rune(str)
	total := len(rs)
	result := make([]rune, 0, total)

	var match bool

	for i := total - 1; i >= 0; i-- {
		if match {
			// check if already escaped
			if rs[i] != '\\' {
				result = append(result, '\\')
			}
			match = false
		} else {
			for _, ec := range escapeChars {
				if rs[i] == ec {
					match = true
					break
				}
			}
		}

		result = append(result, rs[i])

		// in case the matching char is at the beginning
		if i == 0 && match {
			result = append(result, '\\')
		}
	}

	// reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

func containsUnescapedChar(str string, ch rune) bool {
	var prev rune

	for _, c := range str {
		if c == ch && prev != '\\' {
			return true
		}

		if c == '\\' && prev == '\\' {
			prev = rune(0) // reset escape sequence
		} else {
			prev = c
		}
	}

	return false
}

// -------------------------------------------------------------------

var _ dbx.Expression = (*opExpr)(nil)

// opExpr defines an expression that contains a raw sql operator string.
type opExpr struct {
	op string
}

// Build converts the expression into a SQL fragment.
//
// Implements [dbx.Expression] interface.
func (e *opExpr) Build(db *dbx.DB, params dbx.Params) string {
	return e.op
}

// -------------------------------------------------------------------

var _ dbx.Expression = (*concatExpr)(nil)

// concatExpr defines an expression that concatenates multiple
// other expressions with a specified separator.
type concatExpr struct {
	separator string
	parts     []dbx.Expression
}

// Build converts the expression into a SQL fragment.
//
// Implements [dbx.Expression] interface.
func (e *concatExpr) Build(db *dbx.DB, params dbx.Params) string {
	if len(e.parts) == 0 {
		return ""
	}

	stringParts := make([]string, 0, len(e.parts))

	for _, p := range e.parts {
		if p == nil {
			continue
		}

		if sql := p.Build(db, params); sql != "" {
			stringParts = append(stringParts, sql)
		}
	}

	// skip extra parenthesis for single concat expression
	if len(stringParts) == 1 &&
		// check for already concatenated raw/plain expressions
		!strings.Contains(strings.ToUpper(stringParts[0]), " AND ") &&
		!strings.Contains(strings.ToUpper(stringParts[0]), " OR ") {
		return stringParts[0]
	}

	return "(" + strings.Join(stringParts, e.separator) + ")"
}

// -------------------------------------------------------------------

var _ dbx.Expression = (*manyVsManyExpr)(nil)

// manyVsManyExpr constructs a multi-match many<->many db where expression.
//
// Expects leftSubQuery and rightSubQuery to return a subquery with a
// single "multiMatchValue" column.
type manyVsManyExpr struct {
	left  *ResolverResult
	right *ResolverResult
	op    fexpr.SignOp
}

// Build converts the expression into a SQL fragment.
//
// Implements [dbx.Expression] interface.
func (e *manyVsManyExpr) Build(db *dbx.DB, params dbx.Params) string {
	if e.left.MultiMatchSubQuery == nil || e.right.MultiMatchSubQuery == nil {
		return "0=1"
	}

	lAlias := "__ml" + security.PseudorandomString(8)
	rAlias := "__mr" + security.PseudorandomString(8)

	whereExpr, buildErr := buildResolversExpr(
		&ResolverResult{
			NullFallback: e.left.NullFallback,
			Identifier:   "[[" + lAlias + ".multiMatchValue]]",
		},
		e.op,
		&ResolverResult{
			NullFallback: e.right.NullFallback,
			Identifier:   "[[" + rAlias + ".multiMatchValue]]",
			// note: the AfterBuild needs to be handled only once and it
			// doesn't matter whether it is applied on the left or right subquery operand
			AfterBuild: dbx.Not, // inverse for the not-exist expression
		},
	)

	if buildErr != nil {
		return "0=1"
	}

	return fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM (%s) {{%s}} LEFT JOIN (%s) {{%s}} WHERE %s)",
		e.left.MultiMatchSubQuery.Build(db, params),
		lAlias,
		e.right.MultiMatchSubQuery.Build(db, params),
		rAlias,
		whereExpr.Build(db, params),
	)
}

// -------------------------------------------------------------------

var _ dbx.Expression = (*manyVsOneExpr)(nil)

// manyVsOneExpr constructs a multi-match many<->one db where expression.
//
// Expects subQuery to return a subquery with a single "multiMatchValue" column.
//
// You can set inverse=false to reverse the condition sides (aka. one<->many).
type manyVsOneExpr struct {
	otherOperand *ResolverResult
	subQuery     dbx.Expression
	op           fexpr.SignOp
	inverse      bool
	nullFallback NullFallbackPreference
}

// Build converts the expression into a SQL fragment.
//
// Implements [dbx.Expression] interface.
func (e *manyVsOneExpr) Build(db *dbx.DB, params dbx.Params) string {
	if e.subQuery == nil {
		return "0=1"
	}

	alias := "__sm" + security.PseudorandomString(8)

	r1 := &ResolverResult{
		NullFallback: e.nullFallback,
		Identifier:   "[[" + alias + ".multiMatchValue]]",
		AfterBuild:   dbx.Not, // inverse for the not-exist expression
	}

	r2 := &ResolverResult{
		Identifier: e.otherOperand.Identifier,
		Params:     e.otherOperand.Params,
	}

	var whereExpr dbx.Expression
	var buildErr error

	if e.inverse {
		whereExpr, buildErr = buildResolversExpr(r2, e.op, r1)
	} else {
		whereExpr, buildErr = buildResolversExpr(r1, e.op, r2)
	}

	if buildErr != nil {
		return "0=1"
	}

	return fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM (%s) {{%s}} WHERE %s)",
		e.subQuery.Build(db, params),
		alias,
		whereExpr.Build(db, params),
	)
}

// emptyCompareOp returns a PG-compatible comparison operator for empty string comparisons.
//
// "IS NOT" is invalid for non-NULL operands in PostgreSQL, so we use "!=" instead.
func emptyCompareOp(eqOp string) string {
	if eqOp == "IS NOT" {
		return "!="
	}
	return eqOp
}
