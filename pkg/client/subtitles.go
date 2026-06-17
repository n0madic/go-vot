package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/n0madic/go-vot/pkg/config"
	"github.com/n0madic/go-vot/pkg/secure"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

// SubtitlesParams configures a GetSubtitles call.
type SubtitlesParams struct {
	URL         string
	VideoID     string
	Host        string
	RequestLang string
	Headers     map[string]string
}

// SubtitleTrack is one available subtitle track (original + translated).
type SubtitleTrack struct {
	Language           string
	URL                string
	TranslatedLanguage string
	TranslatedURL      string
}

// SubtitlesResult is the outcome of a GetSubtitles call.
type SubtitlesResult struct {
	Waiting   bool
	Subtitles []SubtitleTrack
}

// GetSubtitles fetches the available subtitle tracks for a video.
func (c *Client) GetSubtitles(ctx context.Context, p SubtitlesParams) (*SubtitlesResult, error) {
	if p.RequestLang == "" {
		p.RequestLang = c.requestLang
	}
	if IsCustomLink(p.URL) {
		return c.getSubtitlesVOT(ctx, p)
	}
	return c.getSubtitlesYA(ctx, p)
}

func (c *Client) getSubtitlesYA(ctx context.Context, p SubtitlesParams) (*SubtitlesResult, error) {
	session, err := c.getSession(ctx, config.SessionModuleVideoTranslation)
	if err != nil {
		return nil, err
	}
	body := (&yaproto.SubtitlesRequest{URL: p.URL, Language: p.RequestLang}).Marshal()
	headers := secure.SecYaHeaders("Vsubs", session, body, paths.videoSubtitles)
	for k, v := range p.Headers {
		headers[k] = v
	}

	data, status, err := c.request(ctx, paths.videoSubtitles, body, headers, http.MethodPost)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &VOTError{Msg: fmt.Sprintf("failed to request video subtitles: HTTP %d", status), Data: string(data)}
	}
	var resp yaproto.SubtitlesResponse
	if err := resp.Unmarshal(data); err != nil {
		return nil, err
	}

	out := &SubtitlesResult{Waiting: resp.Waiting}
	for _, s := range resp.Subtitles {
		out.Subtitles = append(out.Subtitles, SubtitleTrack{
			Language:           s.Language,
			URL:                s.URL,
			TranslatedLanguage: s.TranslatedLanguage,
			TranslatedURL:      s.TranslatedURL,
		})
	}
	return out, nil
}

// votSubtitle is the JSON shape returned by the VOT backend subtitles endpoint.
type votSubtitle struct {
	Lang        string `json:"lang"`
	LangFrom    string `json:"lang_from"`
	SubtitleURL string `json:"subtitle_url"`
}

func (c *Client) getSubtitlesVOT(ctx context.Context, p SubtitlesParams) (*SubtitlesResult, error) {
	svc, vid := convertVOT(p.Host, p.VideoID, p.URL)
	var data []votSubtitle
	status, err := c.requestVOT(ctx, paths.videoSubtitles, map[string]any{
		"provider": "yandex",
		"service":  svc,
		"video_id": vid,
	}, p.Headers, &data)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, &VOTError{Msg: fmt.Sprintf("failed to request video subtitles: HTTP %d", status)}
	}

	out := &SubtitlesResult{}
	for _, sub := range data {
		if sub.LangFrom == "" {
			continue
		}
		// Find the original (untranslated) track this one was translated from.
		var orig *votSubtitle
		for i := range data {
			if data[i].Lang == sub.LangFrom {
				orig = &data[i]
				break
			}
		}
		if orig == nil {
			continue
		}
		out.Subtitles = append(out.Subtitles, SubtitleTrack{
			Language:           orig.Lang,
			URL:                orig.SubtitleURL,
			TranslatedLanguage: sub.Lang,
			TranslatedURL:      sub.SubtitleURL,
		})
	}
	return out, nil
}
