package goforge

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type httpError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func RespondError(w http.ResponseWriter, err error) error {
	if forgeErr, ok := errors.AsType[*Error](err); ok {
		return RespondJSON(w, httpError{
			Code:    forgeErr.Code,
			Message: forgeErr.Message,
		}, forgeErr.HTTPStatus)
	}

	return RespondJSON(w, httpError{
		Code:    "ERR_INTERNAL_SERVER_ERROR",
		Message: "Internal server error",
	}, http.StatusInternalServerError)
}

func RespondJSON[T any](w http.ResponseWriter, data T, statusCode int) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	_, err = w.Write(raw)
	if err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	return nil
}
