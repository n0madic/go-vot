package client

import (
	"context"
	"net/http"

	"github.com/n0madic/go-vot/pkg/config"
	"github.com/n0madic/go-vot/pkg/secure"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

// StreamParams configures a TranslateStream call.
type StreamParams struct {
	URL          string
	RequestLang  string
	ResponseLang string
	Headers      map[string]string
}

// StreamResult is the outcome of a stream translation request.
type StreamResult struct {
	Translated bool
	Interval   yaproto.StreamInterval
	PingID     int32
	URL        string
	Timestamp  string
	Message    string
}

// PingStream keeps a live stream translation alive.
func (c *Client) PingStream(ctx context.Context, pingID int32, headers map[string]string) error {
	session, err := c.getSession(ctx, config.SessionModuleVideoTranslation)
	if err != nil {
		return err
	}
	body := (&yaproto.StreamPingRequest{PingID: pingID}).Marshal()
	hdrs := secure.SecYaHeaders("Vtrans", session, body, paths.streamPing)
	for k, v := range headers {
		hdrs[k] = v
	}
	_, ok, err := c.request(ctx, paths.streamPing, body, hdrs, http.MethodPost)
	if err != nil {
		return err
	}
	if !ok {
		return &VOTError{Msg: "failed to request stream ping"}
	}
	return nil
}

// TranslateStream requests a live stream translation.
func (c *Client) TranslateStream(ctx context.Context, p StreamParams) (*StreamResult, error) {
	if p.RequestLang == "" {
		p.RequestLang = c.requestLang
	}
	if p.ResponseLang == "" {
		p.ResponseLang = c.responseLang
	}
	if IsCustomLink(p.URL) {
		return nil, &VOTError{Msg: "unsupported video URL for getting stream translation"}
	}

	session, err := c.getSession(ctx, config.SessionModuleVideoTranslation)
	if err != nil {
		return nil, err
	}
	body := (&yaproto.StreamTranslationRequest{
		URL:              p.URL,
		Language:         p.RequestLang,
		ResponseLanguage: p.ResponseLang,
		Unknown0:         1,
		Unknown1:         0,
	}).Marshal()
	hdrs := secure.SecYaHeaders("Vtrans", session, body, paths.streamTranslation)
	for k, v := range p.Headers {
		hdrs[k] = v
	}

	data, ok, err := c.request(ctx, paths.streamTranslation, body, hdrs, http.MethodPost)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &VOTError{Msg: "failed to request stream translation", Data: string(data)}
	}

	var resp yaproto.StreamTranslationResponse
	if err := resp.Unmarshal(data); err != nil {
		return nil, err
	}

	switch resp.Interval {
	case yaproto.StreamNoConnection, yaproto.StreamTranslating:
		msg := "translationTakeFewMinutes"
		if resp.Interval == yaproto.StreamNoConnection {
			msg = "streamNoConnectionToServer"
		}
		return &StreamResult{Translated: false, Interval: resp.Interval, Message: msg}, nil
	case yaproto.StreamStreaming:
		r := &StreamResult{Translated: true, Interval: resp.Interval}
		if resp.PingID != nil {
			r.PingID = *resp.PingID
		}
		if resp.TranslatedInfo != nil {
			r.URL = resp.TranslatedInfo.URL
			r.Timestamp = resp.TranslatedInfo.Timestamp
		}
		return r, nil
	default:
		return nil, &VOTError{Msg: "unknown response from yandex", Data: string(data)}
	}
}
