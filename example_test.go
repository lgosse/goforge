package goforge_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/lgosse/goforge"
)

func ExampleNewError() {
	err := goforge.NewError(errors.New("user does not exist")).
		WithHTTPStatus(http.StatusNotFound).
		WithCode("ERR_USER_NOT_FOUND").
		WithMessage("User not found")

	fmt.Println(err.HTTPStatus, err.Code, err.Message)
	// Output:
	// 404 ERR_USER_NOT_FOUND User not found
}

func ExampleRespondJSON() {
	recorder := httptest.NewRecorder()

	err := goforge.RespondJSON(recorder, struct {
		ID string `json:"id"`
	}{ID: "user-1"}, http.StatusCreated)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(recorder.Code)
	fmt.Println(recorder.Header().Get("Content-Type"))
	fmt.Println(recorder.Body.String())
	// Output:
	// 201
	// application/json
	// {"id":"user-1"}
}
