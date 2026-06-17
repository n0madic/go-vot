package vot

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/n0madic/go-vot/pkg/client"
	"github.com/n0madic/go-vot/pkg/service"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

// fakeDoer is a programmable http.RoundTripper-like stub satisfying client.Doer
// and service.Fetcher (both need only Do).
type fakeDoer struct {
	fn func(*http.Request) (*http.Response, error)
}

func (f fakeDoer) Do(r *http.Request) (*http.Response, error) { return f.fn(r) }

func makeResp(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(string(body))),
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

func TestShouldRetryWithoutLively(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"lively-specific message", &client.VOTError{Msg: "yandex couldn't translate video", Data: "только обычная озвучка"}, true},
		{"generic couldn't translate", &client.VOTError{Msg: "yandex couldn't translate video"}, true},
		{"msg in Msg field", &client.VOTError{Msg: "ошибка: обычная озвучка недоступна"}, true},
		{"unrelated VOTError", &client.VOTError{Msg: "audio link wasn't received", Data: "boom"}, false},
		{"non-VOTError", errors.New("network down"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldRetryWithoutLively(c.err); got != c.want {
				t.Errorf("shouldRetryWithoutLively(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// TestTranslateLivelyFallback drives Translate fully offline: the first translate
// fails with a cloning-rejected message, the facade retries with the normal
// voice (no sleep, just continue) and succeeds on the second request.
func TestTranslateLivelyFallback(t *testing.T) {
	const outURL = "https://audio.example/out.mp3"
	var translateAuth []string // Authorization header captured per translate call

	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/session/create":
			return makeResp(200, sessionRespBytes("sk", 3600)), nil
		case "/video-translation/translate":
			translateAuth = append(translateAuth, r.Header.Get("Authorization"))
			if len(translateAuth) == 1 {
				// First attempt (lively voice): Yandex rejects cloning.
				msg := "только обычная озвучка доступна для этого видео"
				body := (&yaproto.VideoTranslationResponse{
					Status:  yaproto.StatusFailed,
					Message: &msg,
				}).Marshal()
				return makeResp(200, body), nil
			}
			url := outURL
			body := (&yaproto.VideoTranslationResponse{
				URL:           &url,
				Status:        yaproto.StatusFinished,
				TranslationID: "tid-2",
			}).Marshal()
			return makeResp(200, body), nil
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			return makeResp(404, nil), nil
		}
	}}

	v, err := New(Options{Doer: doer, APIToken: "oauth-tok"})
	if err != nil {
		t.Fatal(err)
	}

	res, err := v.Translate(context.Background(), "https://youtu.be/abc", TranslateOptions{
		RequestLang:    "en",
		ResponseLang:   "ru",
		UseLivelyVoice: true,
	})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if res.URL != outURL {
		t.Errorf("URL = %q, want %q", res.URL, outURL)
	}
	if len(translateAuth) != 2 {
		t.Fatalf("translate called %d times, want 2", len(translateAuth))
	}
	// First request used the lively voice (OAuth attached); the fallback dropped it.
	if translateAuth[0] == "" {
		t.Errorf("first translate had no Authorization header, expected lively OAuth")
	}
	if translateAuth[1] != "" {
		t.Errorf("fallback translate sent Authorization %q, want none (normal voice)", translateAuth[1])
	}
}

func TestToProtoHelp(t *testing.T) {
	if got := toProtoHelp(nil); got != nil {
		t.Errorf("toProtoHelp(nil) = %v, want nil", got)
	}
	in := []service.TranslationHelp{
		{Target: "video_file_url", TargetURL: "https://v/1.mp4"},
		{Target: "subtitles_file_url", TargetURL: "https://v/1.vtt"},
	}
	got := toProtoHelp(in)
	if len(got) != 2 || got[0].Target != "video_file_url" || got[1].TargetURL != "https://v/1.vtt" {
		t.Fatalf("toProtoHelp(%v) = %+v", in, got)
	}
}

func TestBuildHTTPClient(t *testing.T) {
	if _, err := buildHTTPClient("http://proxy.local:8080", 0); err != nil {
		t.Errorf("valid proxy: unexpected error %v", err)
	}
	if _, err := buildHTTPClient("://bad", 0); err == nil {
		t.Error("broken proxy: expected an error, got nil")
	}
}
