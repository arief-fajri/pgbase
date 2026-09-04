// Package mails implements various helper methods for sending common
// emails like forgotten password, verification, etc.
package mails

import (
	"bytes"
	"html/template"
)

// resolveTemplateContent resolves inline html template strings.
//
// html/template is used (instead of text/template) so that interpolated user
// data (record fields, names, tokens) is HTML-escaped in the email body, which
// prevents content/HTML injection into outgoing mail (PGB-L08).
func resolveTemplateContent(data any, content ...string) (string, error) {
	if len(content) == 0 {
		return "", nil
	}

	t := template.New("inline_template")

	var parseErr error
	for _, v := range content {
		t, parseErr = t.Parse(v)
		if parseErr != nil {
			return "", parseErr
		}
	}

	var wr bytes.Buffer

	if executeErr := t.Execute(&wr, data); executeErr != nil {
		return "", executeErr
	}

	return wr.String(), nil
}
