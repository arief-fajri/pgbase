package core

import (
	"slices"
	"testing"
)

func TestRemovedIdentityFields(t *testing.T) {
	scenarios := []struct {
		name      string
		oldFields []string
		newFields []string
		expected  []string
	}{
		{
			"no change",
			[]string{"email", "username"},
			[]string{"email", "username"},
			[]string{},
		},
		{
			"username removed",
			[]string{"email", "username"},
			[]string{"email"},
			[]string{"username"},
		},
		{
			"both removed",
			[]string{"email", "username", "custom_id"},
			[]string{"email"},
			[]string{"username", "custom_id"},
		},
		{
			"new field added - nothing removed",
			[]string{"email"},
			[]string{"email", "phone"},
			[]string{},
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			oldCol := NewBaseCollection("test")
			oldCol.PasswordAuth.IdentityFields = s.oldFields
			newCol := NewBaseCollection("test")
			newCol.PasswordAuth.IdentityFields = s.newFields

			got := removedIdentityFields(oldCol, newCol)

			if len(got) != len(s.expected) {
				t.Fatalf("expected %d removed fields %v, got %v", len(s.expected), s.expected, got)
			}
			for _, f := range s.expected {
				if !slices.Contains(got, f) {
					t.Fatalf("expected removed field %q in %v", f, got)
				}
			}
		})
	}
}
