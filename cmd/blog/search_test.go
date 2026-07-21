package main

import "testing"

func TestTagFilter(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{
			name: "plain tag",
			tag:  "selfhost",
			want: `tags = "selfhost"`,
		},
		{
			name: "apostrophe",
			tag:  "selfhost'",
			want: `tags = "selfhost'"`,
		},
		{
			name: "filter syntax",
			tag:  `selfhost" OR publishedYear = 2026`,
			want: `tags = "selfhost\" OR publishedYear = 2026"`,
		},
		{
			name: "backslash",
			tag:  `self\host`,
			want: `tags = "self\host"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tagFilter(tt.tag); got != tt.want {
				t.Errorf("tagFilter(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}
