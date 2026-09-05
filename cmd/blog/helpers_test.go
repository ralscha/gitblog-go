package main

import (
	"testing"
	"time"

	"github.com/speps/go-hashids/v2"
)

func TestValidFeedbackToken(t *testing.T) {
	hd := hashids.NewData()
	hd.Salt = "test-secret"
	hashID, err := hashids.NewWithData(hd)
	if err != nil {
		t.Fatal(err)
	}
	app := &application{hashID: hashID}
	now := time.Unix(2_000_000_000, 0)

	tests := []struct {
		name    string
		created time.Time
		want    bool
	}{
		{name: "old enough", created: now.Add(-feedbackTokenMinAge), want: true},
		{name: "too new", created: now.Add(-feedbackTokenMinAge + time.Second), want: false},
		{name: "maximum age", created: now.Add(-feedbackTokenMaxAge), want: true},
		{name: "expired", created: now.Add(-feedbackTokenMaxAge - time.Second), want: false},
		{name: "future", created: now.Add(time.Second), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := hashID.Encode([]int{int(tt.created.Unix())})
			if err != nil {
				t.Fatal(err)
			}
			if got := app.validFeedbackToken(token, now); got != tt.want {
				t.Fatalf("validFeedbackToken() = %v, want %v", got, tt.want)
			}
		})
	}

	if app.validFeedbackToken("not-a-token", now) {
		t.Fatal("invalid token was accepted")
	}
}

func TestAbsoluteBlogURL(t *testing.T) {
	if got := absoluteBlogURL("https://example.com/", "/posts/one.html"); got != "https://example.com/posts/one.html" {
		t.Fatalf("absoluteBlogURL() = %q", got)
	}
}

func TestYearNavigationDoesNotAliasInput(t *testing.T) {
	years := []int{2026, 2025}
	navigation := yearNavigation(years, 2025)
	if navigation[0].Current || !navigation[1].Current {
		t.Fatalf("unexpected navigation: %#v", navigation)
	}
}
