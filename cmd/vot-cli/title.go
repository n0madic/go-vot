package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/n0madic/go-vot/pkg/service"
)

// resolveTitle finds a human-readable title for naming output files, trying, in
// order: the resolved VideoData title (already present for some services),
// yt-dlp (covers every yt-dlp-supported site), the YouTube oEmbed endpoint, and
// finally the video id as a fallback.
func resolveTitle(ctx context.Context, hc *http.Client, link string, vd *service.VideoData) string {
	if vd.Title != "" {
		return vd.Title
	}
	if t := ytdlpTitle(ctx, link); t != "" {
		return t
	}
	if t := youtubeOEmbedTitle(ctx, hc, vd); t != "" {
		return t
	}
	return vd.VideoID
}

// ytdlpSourceURL returns the cleanest URL to hand to yt-dlp. For YouTube it
// rebuilds the canonical watch URL from the video id, dropping playlist/index/
// tracking params (e.g. &list=, &index=, &pp=) that can make yt-dlp pick a
// gated format and fail with HTTP 403. Other services use the original link.
func ytdlpSourceURL(vd *service.VideoData, fallback string) string {
	if isYouTubeFamily(vd.Host) && vd.VideoID != "" {
		return "https://www.youtube.com/watch?v=" + vd.VideoID
	}
	return fallback
}

// isYouTubeFamily reports whether the host serves YouTube content (and thus
// supports the YouTube oEmbed endpoint).
func isYouTubeFamily(host string) bool {
	switch host {
	case "youtube", "invidious", "piped", "poketube", "ricktube":
		return true
	}
	return false
}

// youtubeOEmbedTitle fetches a YouTube video title via the public oEmbed endpoint
// (no API key required). Returns "" on any failure.
func youtubeOEmbedTitle(ctx context.Context, hc *http.Client, vd *service.VideoData) string {
	if !isYouTubeFamily(vd.Host) {
		return ""
	}
	api := "https://www.youtube.com/oembed?format=json&url=" + url.QueryEscape(vd.URL)
	raw, err := fetchBytes(ctx, hc, api)
	if err != nil {
		return ""
	}
	var data struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	return data.Title
}
