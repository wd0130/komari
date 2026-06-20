package log

import "testing"

func TestSanitizeRawQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "keeps normal query values",
			in:   "foo=bar&page=1",
			want: "foo=bar&page=1",
		},
		{
			name: "redacts token",
			in:   "token=secret&foo=bar",
			want: "foo=bar&token=REDACTED",
		},
		{
			name: "redacts authorization case insensitive",
			in:   "Authorization=secret",
			want: "Authorization=REDACTED",
		},
		{
			name: "redacts repeated token values",
			in:   "token=one&token=two",
			want: "token=REDACTED&token=REDACTED",
		},
		{
			name: "fallback redacts malformed query",
			in:   "token=%zz&foo=bar",
			want: "token=REDACTED&foo=bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeRawQuery(tt.in); got != tt.want {
				t.Fatalf("sanitizeRawQuery(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
