// Package vot is the high-level facade tying service detection and the VOT API
// client together. It resolves a video URL, requests a translation and polls
// until the translation is ready.
package vot

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/n0madic/go-vot/pkg/client"
	"github.com/n0madic/go-vot/pkg/service"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

// shouldRetryWithoutLively reports whether a failed cloning translation should
// be retried with the normal voice. The userscript only keys off the specific
// "обычная озвучка" message, but Yandex also rejects cloning for some videos
// with a generic "couldn't translate" error — in both cases the normal voice is
// the right fallback, so we trigger on any FAILED translation here.
func shouldRetryWithoutLively(err error) bool {
	var ve *client.VOTError
	if !errors.As(err, &ve) {
		return false
	}
	msg := strings.ToLower(ve.Msg)
	if s, ok := ve.Data.(string); ok {
		msg += " " + strings.ToLower(s)
	}
	return strings.Contains(msg, "обычная озвучка") ||
		strings.Contains(msg, "couldn't translate video")
}

// Options configures a Client.
type Options struct {
	RequestLang  string
	ResponseLang string
	Proxy        string
	Timeout      time.Duration
	Worker       bool
	APIToken     string
	// Doer overrides the HTTP client. When set it is shared as both the API
	// client transport and the service data fetcher (a single Do method
	// satisfies both interfaces), and Proxy/Timeout are ignored. Mainly a test
	// seam for driving Translate/GetVideoData offline.
	Doer client.Doer
}

// Client is the high-level VOT translator.
type Client struct {
	c       *client.Client
	fetcher service.Fetcher
}

// New builds a Client. A single HTTP client (honoring Proxy/Timeout) is shared
// between the API client and the service data fetcher. When opts.Doer is set it
// is used for both instead, bypassing Proxy/Timeout.
func New(opts Options) (*Client, error) {
	var doer client.Doer
	var fetcher service.Fetcher
	if opts.Doer != nil {
		// A single Do(*http.Request) (*http.Response, error) satisfies both the
		// client.Doer and service.Fetcher interfaces.
		doer = opts.Doer
		fetcher = opts.Doer
	} else {
		httpClient, err := buildHTTPClient(opts.Proxy, opts.Timeout)
		if err != nil {
			return nil, err
		}
		doer = httpClient
		fetcher = httpClient
	}
	c, err := client.New(client.Options{
		Doer:         doer,
		RequestLang:  opts.RequestLang,
		ResponseLang: opts.ResponseLang,
		Worker:       opts.Worker,
		APIToken:     opts.APIToken,
	})
	if err != nil {
		return nil, err
	}
	return &Client{c: c, fetcher: fetcher}, nil
}

func buildHTTPClient(proxy string, timeout time.Duration) (*http.Client, error) {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	tr := &http.Transport{}
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Timeout: timeout, Transport: tr}, nil
}

// Raw exposes the underlying low-level API client.
func (v *Client) Raw() *client.Client { return v.c }

// GetVideoData resolves the metadata for a video URL.
func (v *Client) GetVideoData(ctx context.Context, rawURL string) (*service.VideoData, error) {
	return service.GetVideoData(ctx, v.fetcher, rawURL)
}

// TranslateOptions configures a Translate call.
type TranslateOptions struct {
	RequestLang     string
	ResponseLang    string
	ForceSourceLang bool
	UseLivelyVoice  bool
	BypassCache     bool
	// OnProgress, if set, is called on every poll while waiting.
	OnProgress func(status string, remainingTime int32)
}

// Result is a completed translation.
type Result struct {
	URL           string
	TranslationID string
	Status        yaproto.VideoTranslationStatus
	VideoData     *service.VideoData
}

// Translate resolves the video, requests a translation and polls until the audio
// is ready or ctx is cancelled. The first wait honors the server's remainingTime
// (capped at 180s); subsequent polls run every 30s.
func (v *Client) Translate(ctx context.Context, rawURL string, opts TranslateOptions) (*Result, error) {
	vd, err := v.GetVideoData(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	params := client.TranslateParams{
		URL:             vd.URL,
		VideoID:         vd.VideoID,
		Host:            vd.Host,
		Duration:        vd.Duration,
		RequestLang:     opts.RequestLang,
		ResponseLang:    opts.ResponseLang,
		TranslationHelp: toProtoHelp(vd.TranslationHelp),
		ExtraOpts: yaproto.TranslationExtraOpts{
			ForceSourceLang: opts.ForceSourceLang,
			UseLivelyVoice:  opts.UseLivelyVoice,
			BypassCache:     opts.BypassCache,
			VideoTitle:      vd.Title,
		},
	}

	livelyFallbackTried := false
	first := true
	for {
		res, err := v.c.TranslateVideo(ctx, params)
		if err != nil {
			// If voice cloning ("lively voice") failed for this video, fall back
			// to the normal voice and retry (as the userscript does).
			if params.ExtraOpts.UseLivelyVoice && !livelyFallbackTried && shouldRetryWithoutLively(err) {
				livelyFallbackTried = true
				params.ExtraOpts.UseLivelyVoice = false
				if opts.OnProgress != nil {
					opts.OnProgress("voice cloning failed, falling back to normal voice", 0)
				}
				first = true
				continue
			}
			return nil, err
		}
		if res.Translated {
			return &Result{URL: res.URL, TranslationID: res.TranslationID, Status: res.Status, VideoData: vd}, nil
		}
		if opts.OnProgress != nil {
			opts.OnProgress(res.Status.String(), res.RemainingTime)
		}

		wait := 30
		if first && res.RemainingTime > 0 {
			wait = int(res.RemainingTime)
			if wait > 180 {
				wait = 180
			}
		}
		first = false

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(wait) * time.Second):
		}
	}
}

// GetSubtitles resolves the video and returns its available subtitle tracks.
func (v *Client) GetSubtitles(ctx context.Context, rawURL, requestLang string) (*client.SubtitlesResult, error) {
	vd, err := v.GetVideoData(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return v.c.GetSubtitles(ctx, client.SubtitlesParams{
		URL:         vd.URL,
		VideoID:     vd.VideoID,
		Host:        vd.Host,
		RequestLang: requestLang,
	})
}

const (
	// subtitlesPollInterval is how long to wait between subtitle polls while the
	// server is still generating them.
	subtitlesPollInterval = 30 * time.Second
	// subtitlesMaxAttempts caps the number of subtitle polls before giving up.
	subtitlesMaxAttempts = 5
)

// GetSubtitlesWait is like GetSubtitles but, when the server reports the
// subtitles are still being generated (Waiting with no tracks yet), it polls
// every ~30s up to a few times (or until ctx is cancelled). onWait, if set, is
// called before each wait. It returns the last result once tracks appear, the
// server stops waiting, or the attempts are exhausted.
func (v *Client) GetSubtitlesWait(ctx context.Context, rawURL, requestLang string, onWait func()) (*client.SubtitlesResult, error) {
	vd, err := v.GetVideoData(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	params := client.SubtitlesParams{
		URL:         vd.URL,
		VideoID:     vd.VideoID,
		Host:        vd.Host,
		RequestLang: requestLang,
	}

	var result *client.SubtitlesResult
	for attempt := 0; attempt < subtitlesMaxAttempts; attempt++ {
		result, err = v.c.GetSubtitles(ctx, params)
		if err != nil {
			return nil, err
		}
		if !result.Waiting || len(result.Subtitles) > 0 {
			return result, nil
		}
		if attempt == subtitlesMaxAttempts-1 {
			break // exhausted: return the last (still-waiting) result
		}
		if onWait != nil {
			onWait()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(subtitlesPollInterval):
		}
	}
	return result, nil
}

func toProtoHelp(h []service.TranslationHelp) []yaproto.TranslationHelp {
	if len(h) == 0 {
		return nil
	}
	out := make([]yaproto.TranslationHelp, len(h))
	for i, x := range h {
		out[i] = yaproto.TranslationHelp{Target: x.Target, TargetURL: x.TargetURL}
	}
	return out
}
