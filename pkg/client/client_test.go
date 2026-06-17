package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/n0madic/go-vot/pkg/config"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

type fakeDoer struct {
	fn func(*http.Request) (*http.Response, error)
}

func (f fakeDoer) Do(r *http.Request) (*http.Response, error) { return f.fn(r) }

func makeResp(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}

func sessionRespBytes(sk string, expires int32) []byte {
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.BytesType)
	b = protowire.AppendString(b, sk)
	b = protowire.AppendTag(b, 2, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(expires))
	return b
}

func TestTranslateVideoFinished(t *testing.T) {
	url := "https://audio.example/out.mp3"
	var translateHeaders http.Header
	var sawSession bool

	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case paths.session:
			sawSession = true
			return makeResp(200, sessionRespBytes("sk-xyz", 3600)), nil
		case paths.videoTranslation:
			translateHeaders = r.Header.Clone()
			body := (&yaproto.VideoTranslationResponse{
				URL:           &url,
				Status:        yaproto.StatusFinished,
				TranslationID: "tid-9",
			}).Marshal()
			return makeResp(200, body), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	}}

	c, err := New(Options{Doer: doer})
	if err != nil {
		t.Fatal(err)
	}

	res, err := c.TranslateVideo(context.Background(), TranslateParams{
		URL:          "https://youtu.be/abc",
		Host:         "youtube",
		ResponseLang: "ru",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawSession {
		t.Error("expected a session to be created")
	}
	if !res.Translated || res.URL != url || res.TranslationID != "tid-9" {
		t.Fatalf("got %+v", res)
	}

	// Verify the security headers were attached to the translate request.
	if got := translateHeaders.Get("Sec-Vtrans-Sk"); got != "sk-xyz" {
		t.Errorf("Sec-Vtrans-Sk = %q", got)
	}
	if sig := translateHeaders.Get("Vtrans-Signature"); len(sig) != 64 {
		t.Errorf("Vtrans-Signature length = %d, want 64 hex chars", len(sig))
	}
	if tok := translateHeaders.Get("Sec-Vtrans-Token"); !strings.Contains(tok, paths.videoTranslation) {
		t.Errorf("Sec-Vtrans-Token = %q, should contain the path", tok)
	}
	if ct := translateHeaders.Get("Content-Type"); ct != "application/x-protobuf" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestTranslateVideoWaiting(t *testing.T) {
	rt := int32(60)
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case paths.session:
			return makeResp(200, sessionRespBytes("sk", 3600)), nil
		case paths.videoTranslation:
			body := (&yaproto.VideoTranslationResponse{
				Status:        yaproto.StatusWaiting,
				RemainingTime: &rt,
				TranslationID: "tid",
			}).Marshal()
			return makeResp(200, body), nil
		}
		return nil, nil
	}}
	c, _ := New(Options{Doer: doer})
	res, err := c.TranslateVideo(context.Background(), TranslateParams{URL: "https://youtu.be/x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Translated || res.RemainingTime != 60 || res.Status != yaproto.StatusWaiting {
		t.Fatalf("got %+v", res)
	}
}

func TestTranslateVideoSessionRequired(t *testing.T) {
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case paths.session:
			return makeResp(200, sessionRespBytes("sk", 3600)), nil
		case paths.videoTranslation:
			body := (&yaproto.VideoTranslationResponse{Status: yaproto.StatusSessionRequired}).Marshal()
			return makeResp(200, body), nil
		}
		return nil, nil
	}}
	c, _ := New(Options{Doer: doer})
	_, err := c.TranslateVideo(context.Background(), TranslateParams{URL: "https://youtu.be/x"})
	if err != ErrSessionRequired {
		t.Fatalf("err = %v, want ErrSessionRequired", err)
	}
}

func TestWorkerWrapsBody(t *testing.T) {
	var gotPath string
	var gotCT string
	var gotBody []byte
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		// Always return a session then a finished translation; both go through
		// the worker envelope, so just return a session-shaped body here for the
		// session path and a translation body otherwise.
		if r.URL.Path == paths.session {
			return makeResp(200, sessionRespBytes("sk", 3600)), nil
		}
		u := "u"
		return makeResp(200, (&yaproto.VideoTranslationResponse{URL: &u, Status: yaproto.StatusFinished, TranslationID: "t"}).Marshal()), nil
	}}
	c, _ := New(Options{Doer: doer, Worker: true})
	if _, err := c.TranslateVideo(context.Background(), TranslateParams{URL: "https://youtu.be/x"}); err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/json" {
		t.Errorf("worker Content-Type = %q, want application/json", gotCT)
	}
	if !bytes.Contains(gotBody, []byte(`"headers"`)) || !bytes.Contains(gotBody, []byte(`"body"`)) {
		t.Errorf("worker body envelope missing headers/body: %s", gotBody)
	}
	_ = gotPath
}

// TestWorkerRequestJSONBodyIsString verifies the worker JSON envelope carries the
// body as a JSON string value (matching upstream VOTWorkerClient.requestJSON),
// not as a nested JSON object.
func TestWorkerRequestJSONBodyIsString(t *testing.T) {
	var gotBody []byte
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		gotBody, _ = io.ReadAll(r.Body)
		return makeResp(200, []byte(`{"status":1}`)), nil
	}}
	c, _ := New(Options{Doer: doer, Worker: true})
	if err := c.RequestVtransFailAudio(context.Background(), "https://youtu.be/x"); err != nil {
		t.Fatal(err)
	}

	var env struct {
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	}
	if err := json.Unmarshal(gotBody, &env); err != nil {
		t.Fatalf("envelope not valid JSON: %v (%s)", err, gotBody)
	}
	if len(env.Body) == 0 || env.Body[0] != '"' {
		t.Fatalf("worker envelope body should be a JSON string, got %s", env.Body)
	}
	var inner string
	if err := json.Unmarshal(env.Body, &inner); err != nil {
		t.Fatalf("body is not a JSON string: %v", err)
	}
	if !strings.Contains(inner, `"video_url"`) || !strings.Contains(inner, "https://youtu.be/x") {
		t.Errorf("inner body = %q, want the fail-audio JSON", inner)
	}
}

// TestGetSessionConcurrentSingleCreate verifies that concurrent callers for the
// same module create exactly one session (no TOCTOU duplicate creation).
func TestGetSessionConcurrentSingleCreate(t *testing.T) {
	var creates int32
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == paths.session {
			atomic.AddInt32(&creates, 1)
			time.Sleep(10 * time.Millisecond) // widen the race window
			return makeResp(200, sessionRespBytes("sk", 3600)), nil
		}
		t.Errorf("unexpected path %s", r.URL.Path)
		return nil, nil
	}}
	c, _ := New(Options{Doer: doer})

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.getSession(context.Background(), config.SessionModuleVideoTranslation); err != nil {
				t.Errorf("getSession: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&creates); got != 1 {
		t.Fatalf("session created %d times, want 1", got)
	}
}
