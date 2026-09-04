package core

import (
	"net/url"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// editorHTMLSanitizer is the allow-list HTML sanitizer applied to editor-field
// values at write time (PGB-M03). It blocks scripts, event handlers, iframes,
// embeds and javascript:/data:text URIs while preserving typical rich-text
// markup. It is based on bluemonday's UGC policy and is NOT a complete HTML
// validator — it is a defense-in-depth control for content that is later
// rendered in admin/browser contexts.
var editorHTMLSanitizer = newEditorHTMLSanitizer()

func newEditorHTMLSanitizer() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	// allow-list URL schemes (standard http/https/mailto are already allowed by
	// UGC; add data: ONLY for inline raster images, never SVG/text/html)
	p.AllowURLSchemeWithCustomPolicy("data", func(u *url.URL) bool {
		// for a data: URI without a net-locator the mediatype lives in Opaque
		if u.Scheme != "data" || u.Opaque == "" {
			return false
		}
		if !strings.HasPrefix(u.Opaque, "image/") {
			return false
		}
		// explicitly exclude SVG - it can carry embedded scripts
		if strings.HasPrefix(u.Opaque, "image/svg+xml") {
			return false
		}
		return strings.HasPrefix(u.Opaque, "image/png") ||
			strings.HasPrefix(u.Opaque, "image/jpeg") ||
			strings.HasPrefix(u.Opaque, "image/gif") ||
			strings.HasPrefix(u.Opaque, "image/webp") ||
			strings.HasPrefix(u.Opaque, "image/bmp")
	})

	return p
}

// sanitizeEditorHTML sanitizes raw HTML editor content with the allow-list
// policy, stripping active content (scripts, event handlers, iframes, embeds,
// unsafe URL schemes). It is safe to be called with any string, including
// empty.
func sanitizeEditorHTML(raw string) string {
	return editorHTMLSanitizer.Sanitize(raw)
}