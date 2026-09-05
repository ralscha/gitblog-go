package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/go-chi/chi/v5"
)

func (app *application) routes() http.Handler {
	mux := chi.NewRouter()
	mux.Use(app.recoverPanic)
	mux.Use(middleware.NoCache)

	mux.With(middleware.RequestSize(1<<20)).Post("/githubCallback", app.githubCallbackHandler)
	mux.With(middleware.RequestSize(64<<10)).Post("/submitFeedback", app.submitFeedbackHandler)
	mux.Get("/feedback/{url}", app.feedbackHandler)
	mux.Get("/healthz", app.healthHandler)
	mux.Get("/", app.indexHandler)
	mux.Get("/index.html", app.indexHandler)

	return mux
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			err := recover()
			if err != nil {
				app.serverError(w, r, fmt.Errorf("panic: %v", err))
			}
		}()

		next.ServeHTTP(w, r)
	})
}
