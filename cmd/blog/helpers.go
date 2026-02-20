package main

import (
	"fmt"
	"net/http"
)

func (app *application) backgroundTask(r *http.Request, fn func() error) {

	app.wg.Go(func() {

		defer func() {
			err := recover()
			if err != nil {
				app.reportServerError(r, fmt.Errorf("%s", err))
			}
		}()

		err := fn()
		if err != nil {
			app.reportServerError(r, err)
		}
	})
}
