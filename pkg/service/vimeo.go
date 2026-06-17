package service

import (
	"context"
	"fmt"
	"strings"
)

// vimeoData resolves a Vimeo video's title/link/duration via the public viewer
// JWT and the Vimeo API. The private-player path is not ported; on any failure
// it falls back to base data (duration resolved server-side). Ported from
// @vot.js/node/helpers/vimeo.js (public branch).
func vimeoData(ctx context.Context, f Fetcher, svc *Service, _ string, videoID string) (*VideoData, error) {
	base := &VideoData{URL: svc.BaseURL + videoID}

	var viewer struct {
		APIURL string `json:"apiUrl"`
		JWT    string `json:"jwt"`
	}
	if err := getJSON(ctx, f, "https://vimeo.com/_next/viewer", nil, &viewer); err != nil || viewer.APIURL == "" {
		return base, nil
	}

	vid := strings.ReplaceAll(videoID, "/", ":")
	apiURL := fmt.Sprintf("https://%s/videos/%s?fields=name,link,description,duration", viewer.APIURL, vid)

	var info struct {
		Name     string  `json:"name"`
		Link     string  `json:"link"`
		Duration float64 `json:"duration"`
	}
	if err := getJSON(ctx, f, apiURL, map[string]string{"Authorization": "jwt " + viewer.JWT}, &info); err != nil || info.Link == "" {
		return base, nil
	}

	return &VideoData{URL: info.Link, Title: info.Name, Duration: info.Duration}, nil
}
