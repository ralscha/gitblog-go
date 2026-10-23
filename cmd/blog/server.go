package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

func (app *application) serveHTTP() error {
	srv := &http.Server{
		Addr:         app.config.HTTP.Port,
		Handler:      app.routes(),
		ErrorLog:     slog.NewLogLogger(app.logger.Handler(), slog.LevelWarn),
		IdleTimeout:  time.Duration(app.config.HTTP.IdleTimeoutInSeconds) * time.Second,
		ReadTimeout:  time.Duration(app.config.HTTP.ReadTimeoutInSeconds) * time.Second,
		WriteTimeout: time.Duration(app.config.HTTP.WriteTimeoutInSeconds) * time.Second,
	}

	quit, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownErrorChan := make(chan error, 1)

	go func() {
		<-quit.Done()

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(app.config.HTTP.DefaultShutdownPeriodInSeconds)*time.Second)
		defer cancel()

		shutdownErrorChan <- app.shutdown(ctx, srv)
	}()

	app.logger.Info("starting server", slog.Group("server", "addr", srv.Addr))

	serveErr := srv.ListenAndServe()
	// A bind/listener failure must also finish cleanup before runServer closes
	// shared resources such as the syntax highlighter.
	stop()
	shutdownErr := <-shutdownErrorChan
	if !errors.Is(serveErr, http.ErrServerClosed) {
		return errors.Join(serveErr, shutdownErr)
	}

	if shutdownErr != nil {
		return shutdownErr
	}

	app.logger.Info("stopped server", slog.Group("server", "addr", srv.Addr))

	return nil
}

func (app *application) shutdown(ctx context.Context, srv *http.Server) error {
	app.logger.Info("stopping scheduled jobs")
	scheduled := app.taskScheduler.Shutdown()
	// Stop accepting HTTP work before waiting for jobs such as the link report.
	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
		return err
	}
	app.logger.Info("completing background tasks")
	background := make(chan struct{})
	go func() {
		app.wg.Wait()
		close(background)
	}()
	for scheduled != nil || background != nil {
		select {
		case <-scheduled:
			scheduled = nil
		case <-background:
			background = nil
		case <-ctx.Done():
			return fmt.Errorf("finish background tasks: %w", ctx.Err())
		}
	}
	return nil
}
