package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/lgosse/goforge"
)

const defaultHTTPErrorCode = "ERR_DOWNSTREAM_HTTP_ERROR"

// RequestOpts contains optional headers and query parameters for Call.
type RequestOpts struct {
	Headers http.Header
	Query   url.Values
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Call builds and executes a JSON HTTP request and decodes its typed response.
func Call[Resp any](
	ctx context.Context,
	client *http.Client,
	method string,
	baseURL string,
	endpoint string,
	reqBody any,
	opts *RequestOpts,
) (*Resp, error) {
	requestURL, err := url.JoinPath(baseURL, endpoint)
	if err != nil {
		return nil, fmt.Errorf("join request URL: %w", err)
	}

	var body io.Reader
	if reqBody != nil {
		rawBody, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(rawBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request: %w", err)
	}

	if opts != nil {
		for header, values := range opts.Headers {
			for _, value := range values {
				req.Header.Add(header, value)
			}
		}

		query := req.URL.Query()
		for key, values := range opts.Query {
			for _, value := range values {
				query.Add(key, value)
			}
		}
		req.URL.RawQuery = query.Encode()
	}
	if reqBody != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, decodeHTTPError(resp)
	}

	var result Resp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if errors.Is(err, io.EOF) {
			return &result, nil
		}
		return nil, fmt.Errorf("decode HTTP response: %w", err)
	}

	return &result, nil
}

func decodeHTTPError(resp *http.Response) *goforge.Error {
	message := http.StatusText(resp.StatusCode)
	if message == "" {
		message = "Downstream HTTP request failed"
	}

	forgeErr := goforge.NewError(fmt.Errorf("downstream HTTP status: %s", resp.Status)).
		WithHTTPStatus(resp.StatusCode).
		WithCode(defaultHTTPErrorCode).
		WithMessage(message)

	var payload errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return forgeErr
	}
	if payload.Code != "" {
		forgeErr = forgeErr.WithCode(payload.Code)
	}
	if payload.Message != "" {
		forgeErr = forgeErr.WithMessage(payload.Message)
	}

	return forgeErr
}
