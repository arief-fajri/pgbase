package tests

import (
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/core"
)

// GetAuthToken generates a valid JWT auth token for the specified user.
func GetAuthToken(t testing.TB, app core.App, collectionName, email string) string {
	t.Helper()
	record, err := app.FindAuthRecordByEmail(collectionName, email)
	if err != nil {
		t.Fatal(err)
	}
	token, err := record.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// GetStaticAuthToken generates a non-refreshable static auth token.
func GetStaticAuthToken(t testing.TB, app core.App, collectionName, email string, duration time.Duration) string {
	t.Helper()
	record, err := app.FindAuthRecordByEmail(collectionName, email)
	if err != nil {
		t.Fatal(err)
	}
	token, err := record.NewStaticAuthToken(duration)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// GetFileToken generates a file token for the specified user.
func GetFileToken(t testing.TB, app core.App, collectionName, email string) string {
	t.Helper()
	record, err := app.FindAuthRecordByEmail(collectionName, email)
	if err != nil {
		t.Fatal(err)
	}
	token, err := record.NewFileToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}
