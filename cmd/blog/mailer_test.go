package main

import "testing"

func TestNewMailerSupportsUnauthenticatedSMTP(t *testing.T) {
	mailer, err := NewMailer("localhost", 2500, "", "", "sender@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if mailer == nil {
		t.Fatal("NewMailer() returned nil")
	}
}

func TestNewMailerRejectsPasswordWithoutUsername(t *testing.T) {
	if _, err := NewMailer("localhost", 2500, "", "password", "sender@example.com"); err == nil {
		t.Fatal("NewMailer() accepted a password without a username")
	}
}
