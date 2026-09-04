package core

import (
	"context"
	"testing"
	"time"
)

// TestWithWriteDeadline verifies the write-path deadline helper: it adds a
// deadline only when the incoming context has none, otherwise it leaves an
// already-bounded context untouched.
func TestWithWriteDeadline(t *testing.T) {
	t.Run("no deadline adds timeout", func(t *testing.T) {
		timeout := 5 * time.Second

		ctx, cancel := withWriteDeadline(context.Background(), timeout)
		defer cancel()

		d, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected a deadline to be set")
		}
		if remaining := time.Until(d); remaining <= 0 || remaining > timeout {
			t.Fatalf("expected remaining in (0, %s], got %s", timeout, remaining)
		}
	})

	t.Run("existing earlier deadline is preserved", func(t *testing.T) {
		parent, cancelParent := context.WithTimeout(context.Background(), time.Second)
		defer cancelParent()

		parentDeadline, _ := parent.Deadline()

		ctx, cancel := withWriteDeadline(parent, 30*time.Second)
		defer cancel()

		d, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected the parent deadline to be kept")
		}
		if !d.Equal(parentDeadline) {
			t.Fatalf("expected deadline %s to be unchanged, got %s", parentDeadline, d)
		}
	})

	t.Run("nil context is treated as background", func(t *testing.T) {
		timeout := 2 * time.Second

		//nolint:staticcheck // intentionally exercising the nil-ctx defensive branch
		ctx, cancel := withWriteDeadline(nil, timeout)
		defer cancel()

		if ctx == nil {
			t.Fatal("expected a non-nil context")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("expected a deadline for a nil (background) context")
		}
	})
}
