package core

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestSendSystemAlert(t *testing.T) {
	testDataDir, err := os.MkdirTemp("", "sendSystemAlert_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDataDir)

	testApp := NewBaseApp(BaseAppConfig{
		DataDir: testDataDir,
	})
	defer testApp.ResetBootstrapState()

	if err := testApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	cleanupTestSuperusers(testApp)
	if err := createTestSuperusers(testApp, 3); err != nil {
		t.Fatal(err)
	}

	superuser, err := testApp.FindAuthRecordByEmail(CollectionNameSuperusers, "test1@example.com")
	if err != nil {
		t.Fatal(err)
	}

	var sendCalls int
	testApp.OnMailerSend().BindFunc(func(e *MailerEvent) error {
		sendCalls++

		if !strings.Contains(e.Message.Subject, "test_subject") {
			t.Fatalf("Missing %q in Message.Subject:\n%s", "test_subject", e.Message.Subject)
		}

		if !strings.Contains(e.Message.HTML, "test_details") {
			t.Fatalf("Missing %q in Message.HTML:\n%s", "test_details", e.Message.HTML)
		}

		if len(e.Message.To) != 1 || e.Message.To[0].Address != "test1@example.com" {
			t.Fatalf("Expected To address %q, got %v", "test1@example.com", e.Message.To)
		}

		return nil
	})

	sendSystemAlert(testApp, superuser, "test_subject", "test_details")

	if sendCalls != 1 {
		t.Fatalf("Expected 1 mail send call, got %d", sendCalls)
	}
}

func TestSendSystemAlertToAllSuperusers(t *testing.T) {
	testDataDir, err := os.MkdirTemp("", "sendSystemAlertToAllSuperusers_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDataDir)

	testApp := NewBaseApp(BaseAppConfig{
		DataDir: testDataDir,
	})
	defer testApp.ResetBootstrapState()

	if err := testApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	cleanupTestSuperusers(testApp)
	if err := createTestSuperusers(testApp, 3); err != nil {
		t.Fatal(err)
	}

	var sendCalls int
	testApp.OnMailerSend().BindFunc(func(e *MailerEvent) error {
		sendCalls++

		if !strings.Contains(e.Message.Subject, "test_subject") {
			t.Fatalf("Missing %q in Message.Subject:\n%s", "test_subject", e.Message.Subject)
		}

		if !strings.Contains(e.Message.HTML, "test_details") {
			t.Fatalf("Missing %q in Message.HTML:\n%s", "test_details", e.Message.HTML)
		}

		return nil
	})

	sendSystemAlertToAllSuperusers(testApp, "test_subject", "test_details")

	if sendCalls != 3 {
		t.Fatalf("Expected 3 mail send calls, got %d", sendCalls)
	}
}

func cleanupTestSuperusers(app App) {
	// remove existing superusers so createTestSuperusers() produces a
	// deterministic set (these tests reuse the shared default schema)
	if _, err := app.DB().NewQuery(`DELETE FROM "_superusers"`).Execute(); err != nil {
		app.Logger().Warn("cleanupTestSuperusers", "error", err.Error())
	}
}

func createTestSuperusers(app App, total int) error {
	superusersCollection, err := app.FindCollectionByNameOrId(CollectionNameSuperusers)
	if err != nil {
		return err
	}

	for i := range total {
		superuser := NewRecord(superusersCollection)
		superuser.SetEmail("test" + strconv.Itoa(i+1) + "@example.com")
		superuser.SetRandomPassword()

		if err := app.Save(superuser); err != nil {
			return err
		}
	}

	return nil
}
