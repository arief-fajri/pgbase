package apis

import (
	"testing"

	"github.com/arief-fajri/pgbase/core"
)

// TestRealtimeAccessCacheKeySkipsAuthForAuthIndependentRules verifies RT-2:
// for rules that do not reference @request.auth, the memoization key must not
// include the auth pointer, so distinct authenticated users share the single
// access-check result per broadcast.
func TestRealtimeAccessCacheKeySkipsAuthForAuthIndependentRules(t *testing.T) {
	rule := "verified = true"

	authA := &core.Record{}
	authB := &core.Record{}

	riA := &core.RequestInfo{Auth: authA}
	riB := &core.RequestInfo{Auth: authB}

	keyA := realtimeAccessCacheKey(&rule, riA)
	keyB := realtimeAccessCacheKey(&rule, riB)

	if keyA != keyB {
		t.Fatalf("expected the memo key to be shared across distinct users for an auth-independent rule\nA: %q\nB: %q", keyA, keyB)
	}
}

// TestRealtimeAccessCacheKeyIncludesAuthWhenRuleReferencesIt verifies that
// rules referencing @request.auth still key on the auth pointer (correctness).
func TestRealtimeAccessCacheKeyIncludesAuthWhenRuleReferencesIt(t *testing.T) {
	rule := "@request.auth.id != ''"

	authA := &core.Record{}
	authB := &core.Record{}

	riA := &core.RequestInfo{Auth: authA}
	riB := &core.RequestInfo{Auth: authB}

	keyA := realtimeAccessCacheKey(&rule, riA)
	keyB := realtimeAccessCacheKey(&rule, riB)

	if keyA == keyB {
		t.Fatal("expected the memo key to remain distinct per auth for auth-dependent rules")
	}

	// and the same pointer must hit the same key (within-broadcast memoization)
	riA2 := &core.RequestInfo{Auth: authA}
	if k := realtimeAccessCacheKey(&rule, riA2); k != keyA {
		t.Fatalf("expected the same auth pointer to memoize identically, got %q vs %q", k, keyA)
	}
}

// TestRealtimeAccessCacheKeyNilRule verifies a nil rule is handled and shared.
func TestRealtimeAccessCacheKeyNilRule(t *testing.T) {
	var rule *string

	a := &core.Record{}
	b := &core.Record{}

	if realtimeAccessCacheKey(rule, &core.RequestInfo{Auth: a}) != realtimeAccessCacheKey(rule, &core.RequestInfo{Auth: b}) {
		t.Fatal("expected a nil rule to produce a shared (auth-independent) key")
	}
}
