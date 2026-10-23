package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codnect.io/chrono"
)

type shutdownTestScheduler struct {
	chrono.TaskScheduler
	done   chan bool
	called chan struct{}
}

func (s shutdownTestScheduler) Shutdown() chan bool {
	if s.called != nil {
		close(s.called)
	}
	return s.done
}

func TestServeHTTPWaitsForCleanupAfterBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	scheduler := shutdownTestScheduler{done: make(chan bool), called: make(chan struct{})}
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), taskScheduler: scheduler}
	app.config.HTTP.Port = listener.Addr().String()
	app.config.HTTP.DefaultShutdownPeriodInSeconds = 1
	done := make(chan error, 1)
	go func() { done <- app.serveHTTP() }()
	select {
	case <-scheduler.called:
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler was not stopped")
	}
	select {
	case err := <-done:
		close(scheduler.done)
		t.Fatalf("returned before scheduled work finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(scheduler.done)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("listener failure was lost")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not finish cleanup")
	}
}

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
