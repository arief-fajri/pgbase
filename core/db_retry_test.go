package core

import (
	"errors"
	"testing"
)

func TestBaseLockRetry(t *testing.T) {
	t.Parallel()

	err := baseLockRetry(func(attempt int) error {
		if attempt != 1 {
			t.Fatalf("Expected attempt 1, got %d", attempt)
		}
		return nil
	}, 5)

	if err != nil {
		t.Fatalf("Expected nil, got %v", err)
	}

	expectedErr := errors.New("test error")
	err = baseLockRetry(func(attempt int) error {
		return expectedErr
	}, 5)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("Expected %v, got %v", expectedErr, err)
	}
}
