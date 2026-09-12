package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codnect.io/chrono"
)

type shutdownTestScheduler struct {
	chrono.TaskScheduler
	done chan bool
}

func (s shutdownTestScheduler) Shutdown() chan bool { return s.done }

func TestShutdownBoundsScheduledAndBackgroundWork(t *testing.T) {
	for _, blocked := range []string{"scheduled", "background", "none"} {
		t.Run(blocked, func(t *testing.T) {
			scheduled := make(chan bool)
			if blocked != "scheduled" {
				close(scheduled)
			} else {
				defer close(scheduled)
			}
			app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), taskScheduler: shutdownTestScheduler{done: scheduled}}
			if blocked == "background" {
				app.wg.Add(1)
				defer app.wg.Done()
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- app.shutdown(ctx, server.Config) }()
			select {
			case err := <-done:
				if blocked == "none" && err != nil {
					t.Fatal(err)
				}
				if blocked != "none" && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("error = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("shutdown ignored its deadline")
			}
			response, err := server.Client().Get(server.URL)
			if err == nil {
				response.Body.Close()
				t.Fatal("server still accepts requests")
			}
		})
	}
}
