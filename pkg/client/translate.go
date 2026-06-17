package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/n0madic/go-vot/pkg/config"
	"github.com/n0madic/go-vot/pkg/secure"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

// TranslateParams configures a single TranslateVideo call.
type TranslateParams struct {
	URL             string
	VideoID         string
	Host            string
	Duration        float64
	RequestLang     string
	ResponseLang    string
	TranslationHelp []yaproto.TranslationHelp
	ExtraOpts       yaproto.TranslationExtraOpts
	Headers         map[string]string
	// ShouldSendFailedAudio defaults to true (nil => true). Only relevant for the
	// YouTube AUDIO_REQUESTED flow.
	ShouldSendFailedAudio *bool
}

// TranslationResult is the outcome of one translate request. When Translated is
// false the caller should wait RemainingTime seconds and retry.
type TranslationResult struct {
	TranslationID string
	Translated    bool
	URL           string
	Status        yaproto.VideoTranslationStatus
	RemainingTime int32
	Message       string
}

var (
	customLinkRe = regexp.MustCompile(`\.(m3u8|m4(a|v)|mpd)`)
	epicCDNRe    = regexp.MustCompile(`^https://cdn\.qstv\.on\.epicgames\.com`)
)

// IsCustomLink reports whether the URL is a direct media link handled by the VOT
// backend instead of the Yandex protobuf API.
func IsCustomLink(u string) bool {
	return customLinkRe.MatchString(u) || epicCDNRe.MatchString(u)
}

// TranslateVideo requests a single translation. Direct media links are routed to
// the VOT backend; everything else goes to the Yandex protobuf API.
func (c *Client) TranslateVideo(ctx context.Context, p TranslateParams) (*TranslationResult, error) {
	if p.RequestLang == "" {
		p.RequestLang = c.requestLang
	}
	if p.ResponseLang == "" {
		p.ResponseLang = c.responseLang
	}
	if IsCustomLink(p.URL) {
		provider := "yandex"
		if p.ExtraOpts.UseLivelyVoice {
			provider = "yandex_lively"
		}
		return c.translateVideoVOT(ctx, p, provider)
	}
	return c.translateVideoYA(ctx, p)
}

func (c *Client) translateVideoYA(ctx context.Context, p TranslateParams) (*TranslationResult, error) {
	duration := p.Duration
	if duration == 0 {
		duration = config.DefaultDuration
	}

	session, err := c.getSession(ctx, config.SessionModuleVideoTranslation)
	if err != nil {
		return nil, err
	}

	body := yaproto.NewVideoTranslationRequest(p.URL, duration, p.RequestLang, p.ResponseLang, p.TranslationHelp, p.ExtraOpts).Marshal()
	headers := secure.SecYaHeaders("Vtrans", session, body, paths.videoTranslation)
	if p.ExtraOpts.UseLivelyVoice && c.apiToken != "" {
		headers["Authorization"] = "OAuth " + c.apiToken
	}
	for k, v := range p.Headers {
		headers[k] = v
	}

	data, ok, err := c.request(ctx, paths.videoTranslation, body, headers, http.MethodPost)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &VOTError{Msg: "failed to request video translation", Data: string(data)}
	}

	var resp yaproto.VideoTranslationResponse
	if err := resp.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("decode translation response: %w", err)
	}

	switch resp.Status {
	case yaproto.StatusFailed:
		msg := "yandex couldn't translate video"
		if m := derefStr(resp.Message); m != "" {
			msg += ": " + m
		}
		return nil, &VOTError{Msg: msg, Data: derefStr(resp.Message)}
	case yaproto.StatusFinished, yaproto.StatusPartContent:
		if resp.URL == nil || *resp.URL == "" {
			return nil, &VOTError{Msg: "audio link wasn't received from yandex response"}
		}
		return &TranslationResult{
			TranslationID: resp.TranslationID,
			Translated:    true,
			URL:           *resp.URL,
			Status:        resp.Status,
			RemainingTime: derefInt32(resp.RemainingTime, -1),
			Message:       derefStr(resp.Message),
		}, nil
	case yaproto.StatusWaiting, yaproto.StatusLongWaiting:
		return &TranslationResult{
			TranslationID: resp.TranslationID,
			Translated:    false,
			Status:        resp.Status,
			RemainingTime: derefInt32(resp.RemainingTime, 0),
			Message:       derefStr(resp.Message),
		}, nil
	case yaproto.StatusAudioRequested:
		shouldSend := p.ShouldSendFailedAudio == nil || *p.ShouldSendFailedAudio
		if strings.HasPrefix(p.URL, "https://youtu.be/") && shouldSend {
			if err := c.RequestVtransFailAudio(ctx, p.URL); err != nil {
				return nil, err
			}
			emptyAudio := &yaproto.AudioBufferObject{FileID: string(yaproto.AudioWebAPIGetAllGeneratingURLsFromIframe)}
			if _, err := c.RequestVtransAudio(ctx, p.URL, resp.TranslationID, emptyAudio, nil, nil); err != nil {
				return nil, err
			}
			no := false
			retry := p
			retry.ShouldSendFailedAudio = &no
			return c.translateVideoYA(ctx, retry)
		}
		return &TranslationResult{
			TranslationID: resp.TranslationID,
			Translated:    false,
			Status:        resp.Status,
			RemainingTime: derefInt32(resp.RemainingTime, -1),
			Message:       derefStr(resp.Message),
		}, nil
	case yaproto.StatusSessionRequired:
		return nil, ErrSessionRequired
	default:
		return nil, &VOTError{Msg: "unknown response from yandex", Data: string(data)}
	}
}

// votTranslateResponse is the JSON shape returned by the VOT backend.
type votTranslateResponse struct {
	Status        string      `json:"status"`
	TranslatedURL string      `json:"translated_url"`
	ID            json.Number `json:"id"`
	RemainingTime int32       `json:"remaining_time"`
	Message       string      `json:"message"`
}

func (c *Client) translateVideoVOT(ctx context.Context, p TranslateParams, provider string) (*TranslationResult, error) {
	svc, vid := convertVOT(p.Host, p.VideoID, p.URL)
	var resp votTranslateResponse
	ok, err := c.requestVOT(ctx, paths.videoTranslation, map[string]any{
		"provider":  provider,
		"service":   svc,
		"video_id":  vid,
		"from_lang": p.RequestLang,
		"to_lang":   p.ResponseLang,
		"raw_video": p.URL,
	}, p.Headers, &resp)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &VOTError{Msg: "failed to request video translation", Data: resp}
	}

	switch resp.Status {
	case "failed":
		msg := "yandex couldn't translate video"
		if resp.Message != "" {
			msg += ": " + resp.Message
		}
		return nil, &VOTError{Msg: msg, Data: resp.Message}
	case "success":
		if resp.TranslatedURL == "" {
			return nil, &VOTError{Msg: "audio link wasn't received from VOT response"}
		}
		return &TranslationResult{
			TranslationID: resp.ID.String(),
			Translated:    true,
			URL:           resp.TranslatedURL,
			Status:        yaproto.StatusFinished,
			RemainingTime: -1,
		}, nil
	case "waiting":
		return &TranslationResult{
			Translated:    false,
			RemainingTime: resp.RemainingTime,
			Status:        yaproto.StatusWaiting,
			Message:       resp.Message,
		}, nil
	default:
		return nil, &VOTError{Msg: "unknown response from VOT backend", Data: resp}
	}
}

// TranslateVideoCache queries whether a translation is already cached.
func (c *Client) TranslateVideoCache(ctx context.Context, p TranslateParams) (*yaproto.VideoTranslationCacheResponse, error) {
	if p.RequestLang == "" {
		p.RequestLang = c.requestLang
	}
	if p.ResponseLang == "" {
		p.ResponseLang = c.responseLang
	}
	duration := p.Duration
	if duration == 0 {
		duration = config.DefaultDuration
	}

	session, err := c.getSession(ctx, config.SessionModuleVideoTranslation)
	if err != nil {
		return nil, err
	}
	body := (&yaproto.VideoTranslationCacheRequest{
		URL:              p.URL,
		Duration:         duration,
		Language:         p.RequestLang,
		ResponseLanguage: p.ResponseLang,
	}).Marshal()
	headers := secure.SecYaHeaders("Vtrans", session, body, paths.videoTranslationCache)
	for k, v := range p.Headers {
		headers[k] = v
	}

	data, ok, err := c.request(ctx, paths.videoTranslationCache, body, headers, http.MethodPost)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &VOTError{Msg: "failed to request video translation cache", Data: string(data)}
	}
	var resp yaproto.VideoTranslationCacheResponse
	if err := resp.Unmarshal(data); err != nil {
		return nil, err
	}
	return &resp, nil
}

// convertVOT maps a (service, videoId, url) tuple to the VOT backend's expected
// service/video_id, ported from @vot.js/core/utils/vot.js.
func convertVOT(service, videoID, rawURL string) (string, string) {
	if service == "patreon" {
		if u, err := url.Parse(rawURL); err == nil {
			return "mux", strings.TrimPrefix(u.Path, "/")
		}
	}
	return service, videoID
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func derefInt32(p *int32, def int32) int32 {
	if p != nil {
		return *p
	}
	return def
}
