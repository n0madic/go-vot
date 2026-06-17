// Package vot is the high-level facade tying service detection and the VOT API
// client together. It resolves a video URL, requests a translation and polls
// until the translation is ready.
package vot

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/n0madic/go-vot/pkg/client"
	"github.com/n0madic/go-vot/pkg/service"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

// Options configures a Client.
type Options struct {
	RequestLang  string
	ResponseLang string
	Proxy        string
	Timeout      time.Duration
	Worker       bool
	APIToken     string
}

// Client is the high-level VOT translator.
type Client struct {
	c       *client.Client
	fetcher service.Fetcher
}

// New builds a Client. A single HTTP client (honoring Proxy/Timeout) is shared
// between the API client and the service data fetcher.
func New(opts Options) (*Client, error) {
	httpClient, err := buildHTTPClient(opts.Proxy, opts.Timeout)
	if err != nil {
		return nil, err
	}
	c, err := client.New(client.Options{
		Doer:         httpClient,
		RequestLang:  opts.RequestLang,
		ResponseLang: opts.ResponseLang,
		Worker:       opts.Worker,
		APIToken:     opts.APIToken,
	})
	if err != nil {
		return nil, err
	}
	return &Client{c: c, fetcher: httpClient}, nil
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

	first := true
	for {
		res, err := v.c.TranslateVideo(ctx, params)
		if err != nil {
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
