package apis

import (
	"net/http"
	"testing"
)

func TestWriteSecurityHeaders(t *testing.T) {
	t.Run("baseline headers always set", func(t *testing.T) {
		h := make(http.Header)
		writeSecurityHeaders(h, false)

		for _, header := range []string{
			"X-XSS-Protection",
			"X-Content-Type-Options",
			"X-Frame-Options",
			"Referrer-Policy",
		} {
			if h.Get(header) == "" {
				t.Errorf("expected header %q to be set", header)
			}
		}

		if got := h.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
			t.Errorf("expected strict Referrer-Policy, got %q", got)
		}

		if h.Get("Strict-Transport-Security") != "" {
			t.Errorf("HSTS must be absent when disabled, got %q", h.Get("Strict-Transport-Security"))
		}
	})

	t.Run("HSTS opt-in", func(t *testing.T) {
		h := make(http.Header)
		writeSecurityHeaders(h, true)

		if got := h.Get("Strict-Transport-Security"); got == "" {
			t.Error("expected HSTS header to be set when enabled")
		}
	})
}

func TestHSTSEnvGating(t *testing.T) {
	t.Setenv(HSTSEnv, "")
	if hstsEnabledFromEnv() {
		t.Fatal("expected unset to disable HSTS")
	}

	t.Setenv(HSTSEnv, "true")
	if !hstsEnabledFromEnv() {
		t.Fatal("expected PB_HSTS=true to enable HSTS")
	}

	t.Setenv(HSTSEnv, "1")
	if !hstsEnabledFromEnv() {
		t.Fatal("expected PB_HSTS=1 to enable HSTS")
	}

	t.Setenv(HSTSEnv, "TRUE")
	if !hstsEnabledFromEnv() {
		t.Fatal("expected PB_HSTS=TRUE (case-insensitive) to enable HSTS")
	}
}
