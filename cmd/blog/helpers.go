package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	feedbackTokenMinAge = 2 * time.Second
	feedbackTokenMaxAge = time.Hour
)

func (app *application) backgroundTask(r *http.Request, fn func() error) {

	app.wg.Go(func() {

		defer func() {
			err := recover()
			if err != nil {
				app.reportServerError(r, fmt.Errorf("panic: %v", err))
			}
		}()

		err := fn()
		if err != nil {
			app.reportServerError(r, err)
		}
	})
}

func absoluteBlogURL(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func (app *application) renderHTML(w http.ResponseWriter, templateName string, data any) error {
	var output bytes.Buffer
	if err := app.templates[templateName].Execute(&output, data); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := output.WriteTo(w)
	return err
}

func (app *application) validFeedbackToken(token string, now time.Time) bool {
	numbers, err := app.hashID.DecodeWithError(token)
	if err != nil || len(numbers) != 1 {
		return false
	}

	created := time.Unix(int64(numbers[0]), 0)
	age := now.Sub(created)
	return age >= feedbackTokenMinAge && age <= feedbackTokenMaxAge
}

func yearNavigation(years []int, current int) []YearNavigation {
	navigation := make([]YearNavigation, len(years))
	for i, year := range years {
		navigation[i] = YearNavigation{Year: year, Current: year == current}
	}
	return navigation
}
