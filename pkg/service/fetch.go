package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/n0madic/go-vot/pkg/config"
)

// doRequest performs an HTTP request with the default User-Agent (overridable via
// headers) and returns the raw response body, erroring on a non-200 status.
func doRequest(ctx context.Context, f Fetcher, method, rawURL string, body []byte, headers map[string]string) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", config.UserAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := f.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return data, nil
}

// getText performs a GET request and returns the response body as a string.
func getText(ctx context.Context, f Fetcher, rawURL string, headers map[string]string) (string, error) {
	data, err := doRequest(ctx, f, http.MethodGet, rawURL, nil, headers)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// getJSON performs a GET request and decodes the JSON response into out.
func getJSON(ctx context.Context, f Fetcher, rawURL string, headers map[string]string, out any) error {
	data, err := doRequest(ctx, f, http.MethodGet, rawURL, nil, headers)
	if err != nil {
		return err
	}
	return decodeJSON(data, out)
}

// postJSON performs a POST request with the given body and decodes the JSON
// response into out.
func postJSON(ctx context.Context, f Fetcher, rawURL string, body []byte, headers map[string]string, out any) error {
	data, err := doRequest(ctx, f, http.MethodPost, rawURL, body, headers)
	if err != nil {
		return err
	}
	return decodeJSON(data, out)
}

// decodeJSON decodes the first JSON value in data into out, tolerating trailing
// bytes after the value (matching the original Decoder.Decode behavior; some
// service responses append extra content after the JSON payload).
func decodeJSON(data []byte, out any) error {
	return json.NewDecoder(bytes.NewReader(data)).Decode(out)
}
