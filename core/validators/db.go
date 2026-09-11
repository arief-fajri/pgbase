package validators

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
)

// pgUniqueDetailKeyRegex extracts the offending column name(s) from a
// PostgreSQL unique-violation error detail, eg. "Key (title)=(abc) already
// exists.", the multi-column "Key (a, b)=(1, 2) already exists." or the
// functional "Key (lower(email))=(a@b.co) already exists.".
//
// The capture is non-greedy and anchored on the "){=(" value separator so that
// a nested functional expression (eg. lower(email)) is captured whole instead
// of being truncated at its inner closing parenthesis.
var pgUniqueDetailKeyRegex = regexp.MustCompile(`(?i)Key \((.+?)\)=\(`)

// UniqueId checks whether a field string id already exists in the specified table.
//
// Example:
//
//	validation.Field(&form.RelId, validation.By(validators.UniqueId(form.app.DB(), "tbl_example"))
func UniqueId(db dbx.Builder, tableName string) validation.RuleFunc {
	return func(value any) error {
		v, _ := value.(string)
		if v == "" {
			return nil // nothing to check
		}

		var foundId string

		err := db.
			Select("id").
			From(tableName).
			Where(dbx.HashExp{"id": v}).
			Limit(1).
			Row(&foundId)

		if (err != nil && !errors.Is(err, sql.ErrNoRows)) || foundId != "" {
			return validation.NewError("validation_invalid_or_existing_id", "The model id is invalid or already exists.")
		}

		return nil
	}
}

// NormalizeUniqueIndexError attempts to convert a
// "unique constraint failed" error into a validation.Errors.
//
// The provided err is returned as it is without changes if:
// - err is nil
// - err is already validation.Errors
// - err is not "unique constraint failed" error
func NormalizeUniqueIndexError(err error, tableOrAlias string, fieldNames []string) error {
	if err == nil {
		return err
	}

	if _, ok := err.(validation.Errors); ok {
		return err
	}

	msg := strings.ToLower(err.Error())

	// PostgreSQL: prefer the structured error detail which reliably names the
	// offending column(s) regardless of the (arbitrary) index/constraint name,
	// eg. DETAIL: "Key (title)=(test2) already exists.".
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		detailCols := map[string]struct{}{}
		if m := pgUniqueDetailKeyRegex.FindStringSubmatch(pgErr.Detail); len(m) == 2 {
			for _, c := range strings.Split(m[1], ",") {
				// normalize functional expressions (eg. lower(email)) back to
				// the bare column so they map to the collection field name
				col := dbutils.NormalizeIndexColumnName(c)
				detailCols[strings.ToLower(col)] = struct{}{}
			}
		}

		if len(detailCols) > 0 {
			normalizedErrs := validation.Errors{}
			for _, name := range fieldNames {
				if _, ok := detailCols[strings.ToLower(name)]; ok {
					normalizedErrs[name] = validation.NewError("validation_not_unique", "Value must be unique")
				}
			}
			if len(normalizedErrs) > 0 {
				return normalizedErrs
			}
		}
	}

	// check for unique constraint failure (SQLite format)
	if strings.Contains(msg, "unique constraint failed") {
		// note: extra space to unify multi-columns lookup
		msg = strings.ReplaceAll(strings.TrimSpace(msg), ",", " ") + " "

		normalizedErrs := validation.Errors{}

		for _, name := range fieldNames {
			// note: extra spaces to exclude table name with suffix matching the current one
			// 		 OR other fields starting with the current field name
			if strings.Contains(msg, strings.ToLower(" "+tableOrAlias+"."+name+" ")) {
				normalizedErrs[name] = validation.NewError("validation_not_unique", "Value must be unique")
			}
		}

		if len(normalizedErrs) > 0 {
			return normalizedErrs
		}
	}

	// check for unique constraint failure (PostgreSQL format)
	// e.g. "duplicate key value violates unique constraint "users_email_idx""
	if strings.Contains(msg, "duplicate key value violates unique constraint") {
		normalizedErrs := validation.Errors{}
		lowerTable := strings.ToLower(tableOrAlias)

		for _, name := range fieldNames {
			lowerName := strings.ToLower(name)
			// match common index/constraint naming schemes:
			//   <table>_<field>, <table>_<field>_idx, <field>_<table>,
			//   <field>_<table>_idx, idx_<table>_<field>, idx_<field>_<table>
			for _, pattern := range []string{
				lowerTable + "_" + lowerName,
				lowerName + "_" + lowerTable,
				"idx_" + lowerName + "_" + lowerTable,
				"idx_" + lowerTable + "_" + lowerName,
			} {
				if strings.Contains(msg, `"`+pattern+`"`) || strings.Contains(msg, `"`+pattern+`_idx"`) {
					normalizedErrs[name] = validation.NewError("validation_not_unique", "Value must be unique")
					break
				}
			}
		}

		if len(normalizedErrs) > 0 {
			return normalizedErrs
		}
	}

	return err
}
