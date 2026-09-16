package tests

import (
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/auth"
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

// RegisterTestProvider registers an OAuth2 provider under the given name and
// automatically unregisters it when the test finishes, preventing test
// pollution and concurrent-map race conditions on auth.Providers.
func RegisterTestProvider(t testing.TB, name string, factory auth.ProviderFactoryFunc) {
	t.Helper()
	auth.RegisterProvider(name, factory)
	t.Cleanup(func() {
		auth.UnregisterProvider(name)
	})
}
