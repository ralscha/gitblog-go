package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := run([]string{"typo"}, logger)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run() error = %v", err)
	}
}

func TestRunRejectsUnexpectedArguments(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := run([]string{"index", "extra"}, logger); err == nil {
		t.Fatal("run() accepted an unexpected argument")
	}
}
