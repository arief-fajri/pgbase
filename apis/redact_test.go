package apis

import "testing"

func TestRedactSensitiveQueryParams(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"no query", "/api/files/abc", "/api/files/abc"},
		{"trailing question mark", "/api/x?", "/api/x?"},
		{"sensitive token", "/api/files/abc?token=secretxyz", "/api/files/abc?token=[redacted]"},
		{"token with equals in value", "/a?token=v%3D1", "/a?token=[redacted]"},
		{"mixed params", "/a?page=2&code=OTPXYZ&sort=-created", "/a?page=2&code=[redacted]&sort=-created"},
		{"uppercase key", "/a?TOKEN=abc", "/a?TOKEN=[redacted]"},
		{"value empty", "/a?token=", "/a?token=[redacted]"},
		{"access_token", "/a?access_token=jwt", "/a?access_token=[redacted]"},
		{"non sensitive preserved", "/a?id=record_1&view=full", "/a?id=record_1&view=full"},
		{"multiple sensitive", "/a?token=a&apikey=b&password=c", "/a?token=[redacted]&apikey=[redacted]&password=[redacted]"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := redactSensitiveQueryParams(c.input); got != c.want {
				t.Fatalf("redactSensitiveQueryParams(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}