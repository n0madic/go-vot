package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

func epicGamesID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	return reFind(`/(\w{3,5})/[^/]+$`, u.Path, 1), nil
}

// epicGamesData resolves an Epic Games Developer Community post to its video
// playlist URL. It reads the post JSON, finds the video block and extracts the
// playlist URL from the embed page. Ported from @vot.js/node/helpers/epicgames.js.
func epicGamesData(ctx context.Context, f Fetcher, _ *Service, _, videoID string) (*VideoData, error) {
	var post struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Blocks      []struct {
			Type    string `json:"type"`
			VideoID string `json:"video_id"`
		} `json:"blocks"`
	}
	if err := getJSON(ctx, f, "https://dev.epicgames.com/community/api/learning/post.json?hash_id="+videoID, nil, &post); err != nil {
		return nil, fmt.Errorf("epicgames %s: %w", videoID, err)
	}

	embedID := ""
	for _, b := range post.Blocks {
		if b.Type == "video" {
			embedID = b.VideoID
			break
		}
	}
	if embedID == "" {
		return nil, fmt.Errorf("epicgames %s: no video block", videoID)
	}

	body, err := getText(ctx, f, "https://dev.epicgames.com/community/api/cms/videos/"+embedID+"/embed.html", nil)
	if err != nil {
		return nil, fmt.Errorf("epicgames %s: %w", videoID, err)
	}
	playlist := reFind(`videoUrl\s?=\s"([^"]+)"`, body, 1)
	if playlist == "" {
		return nil, fmt.Errorf("epicgames %s: playlist url not found", videoID)
	}
	playlist = strings.Replace(playlist, "qsep://", "https://", 1)
	return &VideoData{URL: playlist, Title: post.Title}, nil
}
