package apis_test

import (
	"testing"

	"github.com/arief-fajri/pgbase/core"
)

func getAuthToken(t testing.TB, app core.App, collectionName, email string) string {
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
