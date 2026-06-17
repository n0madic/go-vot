// Package client implements the low-level Yandex VOT API client, ported from
// @vot.js/core (MinimalClient + VOTClient) and @vot.js/ext (VOTWorkerClient).
//
// A Client performs single API calls (translate, subtitles, audio, stream).
// Polling/retry logic lives in the higher-level pkg/vot facade.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/n0madic/go-vot/pkg/config"
	"github.com/n0madic/go-vot/pkg/secure"
)

// ErrSessionRequired is returned when Yandex demands authentication that this
// client cannot provide.
var ErrSessionRequired = errors.New("yandex auth required to translate video")

// VOTError carries an error message plus the raw response data for diagnostics.
type VOTError struct {
	Msg  string
	Data any
}

func (e *VOTError) Error() string { return e.Msg }

// Doer is the minimal HTTP interface the client needs. *http.Client satisfies it.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

var hostSchemaRe = regexp.MustCompile(`^(https?)://`)

// Options configures a Client.
type Options struct {
	// Host is the Yandex API host (default: api.browser.yandex.ru, or the worker
	// host when Worker is true).
	Host string
	// HostVOT is the FOSWLY VOT backend host (default: vot.toil.cc/v1).
	HostVOT string
	// Doer overrides the HTTP client. When nil a default client is built using
	// Timeout and Proxy.
	Doer Doer
	// Timeout for the default HTTP client (default: 60s). Ignored if Doer is set.
	Timeout time.Duration
	// Proxy is an optional proxy URL for the default HTTP client.
	Proxy string
	// Headers are extra default headers merged into every Yandex request.
	Headers map[string]string
	// APIToken enables the "Lively Voice" premium feature (optional).
	APIToken string
	// RequestLang is the default source language (default: "en").
	RequestLang string
	// ResponseLang is the default target language (default: "ru").
	ResponseLang string
	// Worker routes requests through the worker proxy (JSON-wrapped bodies).
	Worker bool
}

// Client talks to the Yandex VOT API.
type Client struct {
	host, schema       string
	hostVOT, schemaVOT string
	doer               Doer
	headers            map[string]string
	headersVOT         map[string]string
	apiToken           string
	requestLang        string
	responseLang       string
	worker             bool

	mu       sync.Mutex
	createMu sync.Mutex
	sessions map[string]secure.Session
}

// paths are the API endpoint paths.
var paths = struct {
	session                   string
	videoTranslation          string
	videoTranslationFailAudio string
	videoTranslationAudio     string
	videoTranslationCache     string
	videoSubtitles            string
	streamPing                string
	streamTranslation         string
}{
	session:                   "/session/create",
	videoTranslation:          "/video-translation/translate",
	videoTranslationFailAudio: "/video-translation/fail-audio-js",
	videoTranslationAudio:     "/video-translation/audio",
	videoTranslationCache:     "/video-translation/cache",
	videoSubtitles:            "/video-subtitles/get-subtitles",
	streamPing:                "/stream-translation/ping-stream",
	streamTranslation:         "/stream-translation/translate-stream",
}

// New creates a Client from the given options.
func New(opts Options) (*Client, error) {
	host := opts.Host
	if host == "" {
		if opts.Worker {
			host = config.HostWorker
		} else {
			host = config.HostYandex
		}
	}
	hostVOT := opts.HostVOT
	if hostVOT == "" {
		hostVOT = config.HostVOT
	}

	doer := opts.Doer
	if doer == nil {
		var err error
		doer, err = defaultDoer(opts.Proxy, opts.Timeout)
		if err != nil {
			return nil, err
		}
	}

	reqLang := opts.RequestLang
	if reqLang == "" {
		reqLang = "en"
	}
	resLang := opts.ResponseLang
	if resLang == "" {
		resLang = "ru"
	}

	hostStr, schema := splitSchema(host)
	hostVOTStr, schemaVOT := splitSchema(hostVOT)

	headers := map[string]string{
		"User-Agent":      config.UserAgent,
		"Accept":          "application/x-protobuf",
		"Accept-Language": "en",
		"Content-Type":    "application/x-protobuf",
		"Pragma":          "no-cache",
		"Cache-Control":   "no-cache",
	}
	for k, v := range opts.Headers {
		headers[k] = v
	}

	return &Client{
		host:         hostStr,
		schema:       schema,
		hostVOT:      hostVOTStr,
		schemaVOT:    schemaVOT,
		doer:         doer,
		headers:      headers,
		headersVOT:   map[string]string{"User-Agent": "vot.js/" + config.LibVersion, "Content-Type": "application/json", "Pragma": "no-cache", "Cache-Control": "no-cache"},
		apiToken:     opts.APIToken,
		requestLang:  reqLang,
		responseLang: resLang,
		worker:       opts.Worker,
		sessions:     map[string]secure.Session{},
	}, nil
}

func splitSchema(host string) (string, string) {
	if m := hostSchemaRe.FindStringSubmatch(host); m != nil {
		return host[len(m[0]):], m[1]
	}
	return host, "https"
}

func defaultDoer(proxy string, timeout time.Duration) (Doer, error) {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	tr := &http.Transport{}
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy %q: %w", proxy, err)
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Timeout: timeout, Transport: tr}, nil
}

// RequestLang returns the default source language.
func (c *Client) RequestLang() string { return c.requestLang }

// ResponseLang returns the default target language.
func (c *Client) ResponseLang() string { return c.responseLang }

// request performs a protobuf request to the Yandex host and returns the raw
// response body and the HTTP status code.
func (c *Client) request(ctx context.Context, path string, body []byte, headers map[string]string, method string) ([]byte, int, error) {
	if method == "" {
		method = http.MethodPost
	}
	endpoint := c.schema + "://" + c.host + path

	var reqBody []byte
	reqHeaders := map[string]string{}
	if c.worker {
		// Worker mode: wrap the real headers and body inside a JSON envelope.
		merged := mergeHeaders(c.headers, headers)
		payload, err := json.Marshal(struct {
			Headers map[string]string `json:"headers"`
			Body    []int             `json:"body"`
		}{Headers: merged, Body: bytesToInts(body)})
		if err != nil {
			return nil, 0, err
		}
		reqBody = payload
		reqHeaders["Content-Type"] = "application/json"
	} else {
		reqBody = body
		reqHeaders = mergeHeaders(c.headers, headers)
	}

	return c.do(ctx, method, endpoint, reqBody, reqHeaders)
}

// requestJSON performs a JSON request to the Yandex host and decodes into out.
func (c *Client) requestJSON(ctx context.Context, path string, body []byte, headers map[string]string, method string, out any) (int, error) {
	if method == "" {
		method = http.MethodPost
	}
	endpoint := c.schema + "://" + c.host + path

	var reqBody []byte
	reqHeaders := map[string]string{}
	if c.worker {
		merged := mergeHeaders(c.headers, map[string]string{"Content-Type": "application/json", "Accept": "application/json"})
		merged = mergeHeaders(merged, headers)
		var inner any
		if body != nil {
			// Upstream VOTWorkerClient wraps the already-stringified JSON body, so
			// the envelope's "body" is a JSON string value, not a nested object.
			inner = string(body)
		}
		payload, err := json.Marshal(struct {
			Headers map[string]string `json:"headers"`
			Body    any               `json:"body"`
		}{Headers: merged, Body: inner})
		if err != nil {
			return 0, err
		}
		reqBody = payload
		reqHeaders["Content-Type"] = "application/json"
		reqHeaders["Accept"] = "application/json"
	} else {
		reqBody = body
		reqHeaders = mergeHeaders(c.headers, map[string]string{"Content-Type": "application/json"})
		reqHeaders = mergeHeaders(reqHeaders, headers)
	}

	data, status, err := c.do(ctx, method, endpoint, reqBody, reqHeaders)
	if err != nil {
		return 0, err
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return status, fmt.Errorf("decode json response: %w", err)
		}
	}
	return status, nil
}

// requestVOT posts JSON to the FOSWLY VOT backend and decodes into out.
func (c *Client) requestVOT(ctx context.Context, path string, body any, headers map[string]string, out any) (int, error) {
	endpoint := c.schemaVOT + "://" + c.hostVOT + path
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	reqHeaders := mergeHeaders(c.headersVOT, headers)
	data, status, err := c.do(ctx, http.MethodPost, endpoint, payload, reqHeaders)
	if err != nil {
		return 0, err
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return status, fmt.Errorf("decode vot response: %w", err)
		}
	}
	return status, nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, body []byte, headers map[string]string) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, rdr)
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.doer.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	return data, resp.StatusCode, nil
}

func mergeHeaders(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func bytesToInts(b []byte) []int {
	out := make([]int, len(b))
	for i, v := range b {
		out[i] = int(v)
	}
	return out
}
