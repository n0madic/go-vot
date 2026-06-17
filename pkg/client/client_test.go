package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

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
