package main

import (
	"fmt"
	"github.com/go-chi/chi/v5/middleware"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (app *application) routes() http.Handler {
	mux := chi.NewRouter()
	mux.Use(middleware.RealIP)

	mux.Use(app.recoverPanic)
	mux.Use(middleware.NoCache)

	mux.Post("/githubCallback", app.githubCallbackHandler)
	mux.Post("/submitFeedback", app.submitFeedbackHandler)
	mux.Get("/feedback/{url}", app.feedbackHandler)
	mux.Get("/", app.indexHandler)
	mux.Get("/index.html", app.indexHandler)

	return mux
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			err := recover()
			if err != nil {
				app.reportServerError(r, fmt.Errorf("%s", err))
			}
		}()

		next.ServeHTTP(w, r)
	})
}
