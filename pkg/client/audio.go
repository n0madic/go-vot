package client

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/n0madic/go-vot/pkg/config"
	"github.com/n0madic/go-vot/pkg/secure"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

// RequestVtransFailAudio tells the backend that local audio download failed so
// it falls back to server-side processing. PUT /video-translation/fail-audio-js.
func (c *Client) RequestVtransFailAudio(ctx context.Context, videoURL string) error {
	body, err := json.Marshal(map[string]string{"video_url": videoURL})
	if err != nil {
		return err
	}
	var resp struct {
		Status int `json:"status"`
	}
	ok, err := c.requestJSON(ctx, paths.videoTranslationFailAudio, body, nil, http.MethodPut, &resp)
	if err != nil {
		return err
	}
	if !ok || resp.Status != 1 {
		return &VOTError{Msg: "failed to request fake video translation fail audio"}
	}
	return nil
}

// RequestVtransAudio uploads translated audio for services that require it.
// Exactly one of audio (a full file) or partial (a chunk) should be non-nil.
// PUT /video-translation/audio.
func (c *Client) RequestVtransAudio(ctx context.Context, videoURL, translationID string, audio *yaproto.AudioBufferObject, partial *yaproto.ChunkAudioObject, headers map[string]string) (*yaproto.VideoTranslationAudioResponse, error) {
	session, err := c.getSession(ctx, config.SessionModuleVideoTranslation)
	if err != nil {
		return nil, err
	}

	req := &yaproto.VideoTranslationAudioRequest{TranslationID: translationID, URL: videoURL}
	if partial != nil {
		req.PartialAudioInfo = partial
	} else {
		req.AudioInfo = audio
	}
	body := req.Marshal()

	hdrs := secure.SecYaHeaders("Vtrans", session, body, paths.videoTranslationAudio)
	for k, v := range headers {
		hdrs[k] = v
	}

	data, ok, err := c.request(ctx, paths.videoTranslationAudio, body, hdrs, http.MethodPut)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &VOTError{Msg: "failed to request video translation audio", Data: string(data)}
	}
	var resp yaproto.VideoTranslationAudioResponse
	if err := resp.Unmarshal(data); err != nil {
		return nil, err
	}
	return &resp, nil
}
