package steps

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// TestContext holds the shared state for a test scenario.
type TestContext struct {
	BaseURL  string
	Response *http.Response
	Body     string
	Client   *http.Client
	Headers  map[string]string
}

// NewTestContext creates a new test context with the configured base URL.
func NewTestContext() *TestContext {
	baseURL := os.Getenv("API_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	return &TestContext{
		BaseURL: baseURL,
		Client:  &http.Client{},
		Headers: map[string]string{},
	}
}

// SetHeader stores a header to be attached to the next request.
func (tc *TestContext) SetHeader(name, value string) error {
	tc.Headers[name] = value
	return nil
}

// SendGETRequest sends a GET request to the given path.
func (tc *TestContext) SendGETRequest(path string) error {
	return tc.send(http.MethodGet, path, "")
}

// SendPOSTRequest sends a POST request with the supplied JSON body.
func (tc *TestContext) SendPOSTRequest(path, body string) error {
	return tc.send(http.MethodPost, path, body)
}

func (tc *TestContext) send(method, path, body string) error {
	url := tc.BaseURL + path

	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to build %s %s: %w", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range tc.Headers {
		req.Header.Set(k, v)
	}

	resp, err := tc.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send %s %s: %w", method, url, err)
	}

	tc.Response = resp
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	tc.Body = string(respBody)
	tc.Headers = map[string]string{}

	return nil
}

// AssertResponseStatus checks that the response status code matches the expected value.
func (tc *TestContext) AssertResponseStatus(expected int) error {
	if tc.Response == nil {
		return fmt.Errorf("no response received")
	}

	if tc.Response.StatusCode != expected {
		return fmt.Errorf("expected status %d, got %d (body: %s)", expected, tc.Response.StatusCode, tc.Body)
	}

	return nil
}

// AssertBodyContains checks that the response body contains the expected string.
func (tc *TestContext) AssertBodyContains(expected string) error {
	if !strings.Contains(tc.Body, expected) {
		return fmt.Errorf("expected body to contain %q, got: %s", expected, tc.Body)
	}

	return nil
}
