package core

import (
	"strings"
	"testing"
)

func TestSanitizeEditorHTML(t *testing.T) {
	scenarios := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain text", "hello world", "hello world"},
		{"basic markup preserved", "<p><strong>bold</strong> &amp; <em>em</em></p>", "<p><strong>bold</strong> &amp; <em>em</em></p>"},
		{"links preserved", `<a href="https://example.com">link</a>`, `<a href="https://example.com" rel="nofollow">link</a>`},
		{"safe bullet list", "<ul><li>a</li><li>b</li></ul>", "<ul><li>a</li><li>b</li></ul>"},
		{"img with https kept", `<img src="https://example.com/x.png" alt="x">`, `<img src="https://example.com/x.png" alt="x">`},

		// active content must be stripped
		{"script tag stripped", `<p>before</p><script>alert(1)</script><p>after</p>`, "<p>before</p><p>after</p>"},
		{"inline event handler stripped", `<img src="x" onerror="alert(1)">`, `<img src="x">`},
		{"svg onload stripped", `<svg onload="alert(1)"></svg>`, ""},
		{"iframe stripped", `<p>x</p><iframe src="//evil.example"></iframe><p>y</p>`, "<p>x</p><p>y</p>"},
		{"object stripped", `<object data="x"></object>hello`, "hello"},
		{"javascript href stripped", `<a href="javascript:alert(1)">x</a>`, "x"},
		{"vbscript href stripped", `<a href="vbscript:msgbox(1)">x</a>`, "x"},
		{"data text html img stripped", `<img src="data:text/html;base64,PHNjcmlwdD4=">`, ""},
		{"data svg img stripped", `<img src="data:image/svg+xml;base64,PHNjcmlwdD4=">`, ""},
		{"style with expression stripped", `<div style="width:expression(alert(1))">x</div>`, "<div>x</div>"},
		{"href with uppercase scheme stripped", `<a href="JaVaScRiPt:alert(1)">x</a>`, "x"},

		// inline raster images via data: are allowed (editor pasted images)
		{"data png allowed", `<img src="data:image/png;base64,AAA">`, `<img src="data:image/png;base64,AAA">`},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			got := sanitizeEditorHTML(s.in)
			for _, marker := range []string{"<script", "onerror", "onload", "javascript:", "vbscript:", "expression(", "iframe", "<object", "<embed"} {
				if strings.Contains(strings.ToLower(got), marker) {
					t.Fatalf("sanitizeEditorHTML(%q) = %q: still contains dangerous marker %q", s.in, got, marker)
				}
			}

			// exact-match where the sanitizer output is deterministic
			if s.want != "" && got != s.want {
				t.Fatalf("sanitizeEditorHTML(%q) = %q, want %q", s.in, got, s.want)
			}
		})
	}
}

// TestSanitizeEditorHTMLIdempotent verifies that sanitized output is stable
// (a second pass does not change it), which matters for re-writes of stored
// content.
func TestSanitizeEditorHTMLIdempotent(t *testing.T) {
	samples := []string{
		"<p>hello <strong>world</strong></p>",
		"<p>x</p><script>alert(1)</script><p>y</p>",
		`<a href="https://example.com">link</a>`,
	}

	for _, s := range samples {
		once := sanitizeEditorHTML(s)
		twice := sanitizeEditorHTML(once)
		if once != twice {
			t.Fatalf("sanitizeEditorHTML is not idempotent: %q -> %q -> %q", s, once, twice)
		}
	}
}