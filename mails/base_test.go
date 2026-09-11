package mails

import (
	"strings"
	"testing"
)

// TestResolveTemplateContentEscapesHTML verifies user-controlled data is HTML-
// escaped in the email body (PGB-L08), so injected markup cannot reach the mail
// client.
func TestResolveTemplateContentEscapesHTML(t *testing.T) {
	src := `<p>Hi {{.name}}, verify: <a href="{{.url}}">link</a></p>`

	out, err := resolveTemplateContent(map[string]any{
		"name": `<script>alert(1)</script>`,
		"url":  `javascript:alert(1)`,
	}, src)
	if err != nil {
		t.Fatalf("resolveTemplateContent: %v", err)
	}

	if strings.Contains(out, "<script>") {
		t.Fatalf("expected rendered name to be HTML-escaped, got: %s", out)
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("expected &lt;script&gt; in output, got: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "javascript:alert") {
		t.Fatalf("expected javascript: URL to be escaped in the href context, got: %s", out)
	}
	if !strings.Contains(out, "Hi ") {
		t.Fatalf("expected template structure to be preserved, got: %s", out)
	}
}
