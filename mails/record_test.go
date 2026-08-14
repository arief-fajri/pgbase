package mails_test

import (
	"html"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/mails"
	"github.com/arief-fajri/pgbase/tests"
)

func TestSendRecordAuthAlert(t *testing.T) {
	t.Parallel()

	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	info := "<p>test_info</p>"

	user, err := testApp.FindFirstRecordByData("users", "email", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	err = mails.SendRecordAuthAlert(testApp, user, info)
	if err != nil {
		t.Fatal(err)
	}

	if testApp.TestMailer.TotalSend() != 1 {
		t.Fatalf("Expected one email to be sent, got %d", testApp.TestMailer.TotalSend())
	}

	expectedParts := []string{
		"login to your " + testApp.Settings().Meta.AppName + " account from a new location",
		"If this was you",
		"If this wasn't you",
		html.EscapeString(info),
	}
	for _, part := range expectedParts {
		if !strings.Contains(testApp.TestMailer.LastMessage().HTML, part) {
			t.Fatalf("Couldn't find %s \nin\n %s", part, testApp.TestMailer.LastMessage().HTML)
		}
	}
}

func TestSendRecordPasswordReset(t *testing.T) {
	t.Parallel()

	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	user, err := testApp.FindFirstRecordByData("users", "email", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	err = mails.SendRecordPasswordReset(testApp, user)
	if err != nil {
		t.Fatal(err)
	}

	if testApp.TestMailer.TotalSend() != 1 {
		t.Fatalf("Expected one email to be sent, got %d", testApp.TestMailer.TotalSend())
	}

	expectedParts := []string{
		"http://localhost:8090/_/#/auth/confirm-password-reset/eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.",
	}
	for _, part := range expectedParts {
		if !strings.Contains(testApp.TestMailer.LastMessage().HTML, part) {
			t.Fatalf("Couldn't find %s \nin\n %s", part, testApp.TestMailer.LastMessage().HTML)
		}
	}
}

func TestSendRecordVerification(t *testing.T) {
	t.Parallel()

	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	user, err := testApp.FindFirstRecordByData("users", "email", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	err = mails.SendRecordVerification(testApp, user)
	if err != nil {
		t.Fatal(err)
	}

	if testApp.TestMailer.TotalSend() != 1 {
		t.Fatalf("Expected one email to be sent, got %d", testApp.TestMailer.TotalSend())
	}

	expectedParts := []string{
		"http://localhost:8090/_/#/auth/confirm-verification/eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.",
	}
	for _, part := range expectedParts {
		if !strings.Contains(testApp.TestMailer.LastMessage().HTML, part) {
			t.Fatalf("Couldn't find %s \nin\n %s", part, testApp.TestMailer.LastMessage().HTML)
		}
	}
}

func TestSendRecordChangeEmail(t *testing.T) {
	t.Parallel()

	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	user, err := testApp.FindFirstRecordByData("users", "email", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	err = mails.SendRecordChangeEmail(testApp, user, "new_test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if testApp.TestMailer.TotalSend() != 1 {
		t.Fatalf("Expected one email to be sent, got %d", testApp.TestMailer.TotalSend())
	}

	expectedParts := []string{
		"http://localhost:8090/_/#/auth/confirm-email-change/eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.",
	}
	for _, part := range expectedParts {
		if !strings.Contains(testApp.TestMailer.LastMessage().HTML, part) {
			t.Fatalf("Couldn't find %s \nin\n %s", part, testApp.TestMailer.LastMessage().HTML)
		}
	}
}

func TestSendRecordOTP(t *testing.T) {
	t.Parallel()

	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer testApp.Cleanup()

	user, err := testApp.FindFirstRecordByData("users", "email", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	err = mails.SendRecordOTP(testApp, user, "test_otp_id", "test_otp_code")
	if err != nil {
		t.Fatal(err)
	}

	if testApp.TestMailer.TotalSend() != 1 {
		t.Fatalf("Expected one email to be sent, got %d", testApp.TestMailer.TotalSend())
	}

	expectedParts := []string{
		"one-time password",
		"test_otp_code",
	}
	for _, part := range expectedParts {
		if !strings.Contains(testApp.TestMailer.LastMessage().HTML, part) {
			t.Fatalf("Couldn't find %s \nin\n %s", part, testApp.TestMailer.LastMessage().HTML)
		}
	}
}
