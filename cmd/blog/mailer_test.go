package main

import "testing"

func TestNewMailerSupportsUnauthenticatedSMTP(t *testing.T) {
	mailer, err := NewMailer("localhost", 2500, "", "", "sender@example.com", "none")
	if err != nil {
		t.Fatal(err)
	}
	if mailer == nil {
		t.Fatal("NewMailer() returned nil")
	}
}

func TestNewMailerRejectsPasswordWithoutUsername(t *testing.T) {
	if _, err := NewMailer("localhost", 2500, "", "password", "sender@example.com", "mandatory"); err == nil {
		t.Fatal("NewMailer() accepted a password without a username")
	}
}

func TestNewMailerTLSConfiguration(t *testing.T) {
	for _, policy := range []string{"", "mandatory", "opportunistic", "none"} {
		mailer, err := NewMailer("localhost", 2500, "", "", "sender@example.com", policy)
		if err != nil {
			t.Fatal(err)
		}
		if policy == "" && mailer.client.TLSPolicy() != "TLSMandatory" {
			t.Fatalf("default TLS policy = %s", mailer.client.TLSPolicy())
		}
	}
	if _, err := NewMailer("localhost", 2500, "", "", "sender@example.com", "typo"); err == nil {
		t.Fatal("invalid TLS policy accepted")
	}
}
