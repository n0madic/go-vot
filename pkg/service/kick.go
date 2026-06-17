package service

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
)

// kickID extracts the video or clip path from a kick.com URL.
// Returns the "videos/<id>" or "clips/<id>" portion. Ported from
// @vot.js/node/helpers/kick.js.
func kickID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	return reFind(`([^/]+)/((videos|clips)/([^/]+))`, u.Path, 2), nil
}

// kickData fetches metadata for a Kick video or clip. The videoID is either
// "videos/<id>" (VOD) or "clips/<id>" (clip). Ported from
// @vot.js/node/helpers/kick.js.
func kickData(ctx context.Context, f Fetcher, _ *Service, _ string, videoID string) (*VideoData, error) {
	if strings.HasPrefix(videoID, "videos/") {
		id := strings.TrimPrefix(videoID, "videos/")
		var resp struct {
			Source     string `json:"source"`
			Livestream struct {
				SessionTitle string  `json:"session_title"`
				Duration     float64 `json:"duration"`
			} `json:"livestream"`
		}
		if err := getJSON(ctx, f, "https://kick.com/api/v1/video/"+id, nil, &resp); err != nil {
			return nil, fmt.Errorf("kick video %s: %w", id, err)
		}
		return &VideoData{
			URL:      resp.Source,
			Title:    resp.Livestream.SessionTitle,
			Duration: math.Round(resp.Livestream.Duration / 1000),
		}, nil
	}

	// clips/<id>
	id := strings.TrimPrefix(videoID, "clips/")
	var resp struct {
		Clip struct {
			ClipURL  string  `json:"clip_url"`
			Duration float64 `json:"duration"`
			Title    string  `json:"title"`
		} `json:"clip"`
	}
	if err := getJSON(ctx, f, "https://kick.com/api/v2/clips/"+id, nil, &resp); err != nil {
		return nil, fmt.Errorf("kick clip %s: %w", id, err)
	}
	return &VideoData{
		URL:      resp.Clip.ClipURL,
		Title:    resp.Clip.Title,
		Duration: resp.Clip.Duration,
	}, nil
}
