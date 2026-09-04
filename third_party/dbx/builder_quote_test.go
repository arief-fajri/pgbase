package dbx

import "testing"

// TestQuoteSimpleIdentifierEscaping verifies embedded double-quotes are escaped
// (PostgreSQL identifier escaping) instead of being passed through verbatim
// (PGB-L01).
func TestQuoteSimpleIdentifierEscaping(t *testing.T) {
	b := &BaseBuilder{}

	if got := b.QuoteSimpleTableName("users"); got != `"users"` {
		t.Fatalf("QuoteSimpleTableName(users) = %q", got)
	}
	if got := b.QuoteSimpleTableName(`usr"bad`); got != `"usr""bad"` {
		t.Fatalf("QuoteSimpleTableName with embedded quote = %q", got)
	}
	if got := b.QuoteSimpleColumnName("name"); got != `"name"` {
		t.Fatalf("QuoteSimpleColumnName(name) = %q", got)
	}
	if got := b.QuoteSimpleColumnName("*"); got != `*` {
		t.Fatalf("QuoteSimpleColumnName(*) = %q", got)
	}
	if got := b.QuoteSimpleColumnName(`na"me`); got != `"na""me"` {
		t.Fatalf("QuoteSimpleColumnName with embedded quote = %q", got)
	}

	// already-quoted identifiers must be normalized, not double-wrapped
	// (RecordQuery pre-quotes before re-quoting - PGB-L01)
	if got := b.QuoteSimpleTableName(`"_authOrigins"`); got != `"_authOrigins"` {
		t.Fatalf("QuoteSimpleTableName already-quoted = %q", got)
	}
	if got := b.QuoteSimpleColumnName(`"col"`); got != `"col"` {
		t.Fatalf("QuoteSimpleColumnName already-quoted = %q", got)
	}
	// an already-quoted identifier with a malicious embedded quote is escaped
	if got := b.QuoteSimpleTableName(`"a"b"`); got != `"a""b"` {
		t.Fatalf("QuoteSimpleTableName already-quoted with inner quote = %q", got)
	}
}