package main

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/go-github/v57/github"
)

func (app *application) githubCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(app.config.Github.WebhookSecret) == "" {
		app.logger.Warn("GitHub webhook is disabled: no secret configured")
		clientError(w, http.StatusServiceUnavailable)
		return
	}

	payload, err := github.ValidatePayload(r, []byte(app.config.Github.WebhookSecret))
	if err != nil {
		app.logger.Warn("rejected GitHub webhook", "error", err)
		clientError(w, http.StatusUnauthorized)
		return
	}
	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		app.logger.Warn("rejected malformed GitHub webhook", "error", err)
		clientError(w, http.StatusBadRequest)
		return
	}

	_, ok := event.(*github.PushEvent)
	if ok {
		app.backgroundTask(r, func() error {
			return app.updatePosts()
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) submitFeedbackHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			clientError(w, http.StatusRequestEntityTooLarge)
		} else {
			clientError(w, http.StatusBadRequest)
		}
		return
	}

	url := strings.TrimSpace(r.PostForm.Get("url"))
	token := r.PostForm.Get("token")
	feedback := strings.TrimSpace(r.PostForm.Get("feedback"))
	email := strings.TrimSpace(r.PostForm.Get("email"))
	name := r.PostForm.Get("name")

	validURL := url != "" && len(url) <= 512 && !strings.ContainsAny(url, "\r\n")
	validFeedback := feedback != "" && len(feedback) <= 20_000
	validEmail := len(email) <= 320
	if validFeedback && validURL && validEmail && name == "" && app.validFeedbackToken(token, time.Now()) {
		app.backgroundTask(r, func() error {
			return app.mailer.SendFeedback(email, url, feedback)
		})
	}

	err := app.renderHTML(w, "feedback_ok", FeedbackPage{SiteTitle: app.config.Blog.Title})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
}

func (app *application) feedbackHandler(w http.ResponseWriter, r *http.Request) {
	url := chi.URLParam(r, "url")
	token, err := app.hashID.Encode([]int{int(time.Now().Unix())})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.renderHTML(w, "feedback", FeedbackPage{
		SiteTitle: app.config.Blog.Title,
		PostURL:   url,
		Token:     token,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
}

func (app *application) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (app *application) indexHandler(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	yearString := strings.TrimSpace(r.URL.Query().Get("year"))

	year := -1
	if yearString != "" {
		var err error
		year, err = strconv.Atoi(yearString)
		if err != nil || year < 1 || year > 9999 {
			clientError(w, http.StatusBadRequest)
			return
		}
	}

	publishedYears := app.searchService.PublishedYears()
	data := SearchResults{
		SiteTitle:       app.config.Blog.Title,
		SiteDescription: app.config.Blog.Description,
	}

	if tag != "" {
		posts, err := app.searchService.SearchWithTag(tag)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		sortPostsNewest(posts)

		data.Posts = posts
		data.Query = "tags:" + tag
		data.Years = yearNavigation(publishedYears, -1)

	} else if query != "" {
		posts, err := app.searchService.Search(query)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		data.Posts = posts
		data.Query = query
		data.Years = yearNavigation(publishedYears, -1)
	} else if year != -1 {
		posts, err := app.searchService.SearchPostsOfYear(year)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		sortPostsNewest(posts)

		data.Posts = posts
		data.Years = yearNavigation(publishedYears, year)
	} else {
		currentYear := time.Now().Year()
		if len(publishedYears) > 0 && !slices.Contains(publishedYears, currentYear) {
			currentYear = publishedYears[0]
		}
		posts, err := app.searchService.SearchPostsOfYear(currentYear)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		sortPostsNewest(posts)

		data.Posts = posts
		data.Years = yearNavigation(publishedYears, currentYear)
	}

	err := app.renderHTML(w, "index", data)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
}
