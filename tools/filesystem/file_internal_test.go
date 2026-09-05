package filesystem

import (
	"strings"
	"testing"
)

func TestCappedBody(t *testing.T) {
	// within the cap
	{
		raw, err := cappedBody(strings.NewReader("hello"), 100)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if string(raw) != "hello" {
			t.Fatalf("Expected %q, got %q", "hello", raw)
		}
	}

	// exactly at the cap
	{
		raw, err := cappedBody(strings.NewReader(strings.Repeat("a", 100)), 100)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(raw) != 100 {
			t.Fatalf("Expected %d bytes, got %d", 100, len(raw))
		}
	}

	// exceeding the cap
	{
		_, err := cappedBody(strings.NewReader(strings.Repeat("a", 101)), 100)
		if err == nil {
			t.Fatal("Expected cap error, got nil")
		}
	}

	// zero cap disallows any non-empty body
	{
		if _, err := cappedBody(strings.NewReader("x"), 0); err == nil {
			t.Fatal("Expected cap error for zero cap, got nil")
		}
	}
}