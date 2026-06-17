package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// bannedVideoQuery is the GraphQL query that resolves a banned.video / madmaxworld
// video to its direct URL, title and duration. Field aliases match the JSON keys
// read back from the response. Ported from @vot.js/node/helpers/bannedvideo.js.
const bannedVideoQuery = `query GetVideo($id: String!) {
  getVideo(id: $id) {
    title
    description: summary
    duration: videoDuration
    videoUrl: directUrl
    isStream: live
  }
}`

func bannedVideoID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	return u.Query().Get("id"), nil
}

func bannedVideoData(ctx context.Context, f Fetcher, _ *Service, _, videoID string) (*VideoData, error) {
	reqBody, err := json.Marshal(map[string]any{
		"operationName": "GetVideo",
		"query":         bannedVideoQuery,
		"variables":     map[string]string{"id": videoID},
	})
	if err != nil {
		return nil, fmt.Errorf("bannedvideo %s: %w", videoID, err)
	}

	var resp struct {
		Data struct {
			GetVideo struct {
				Title    string  `json:"title"`
				VideoURL string  `json:"videoUrl"`
				Duration float64 `json:"duration"`
			} `json:"getVideo"`
		} `json:"data"`
	}
	if err := postJSON(ctx, f, "https://api.banned.video/graphql", reqBody, map[string]string{
		"User-Agent":                   "bannedVideoFrontEnd",
		"apollographql-client-name":    "banned-web",
		"apollographql-client-version": "1.3",
		"content-type":                 "application/json",
	}, &resp); err != nil {
		return nil, fmt.Errorf("bannedvideo %s: %w", videoID, err)
	}
	v := resp.Data.GetVideo
	if v.VideoURL == "" {
		return nil, fmt.Errorf("bannedvideo %s: empty video url", videoID)
	}
	return &VideoData{URL: v.VideoURL, Title: v.Title, Duration: v.Duration}, nil
}
