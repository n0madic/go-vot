package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// This file groups services whose data resolution is a single page fetch plus a
// regex over the HTML. Ported from @vot.js/node/helpers/{odysee,appledeveloper,
// bitview}.js. go-vot returns the raw media URL (upstream wraps some in
// media-proxy; not ported, consistent with the vimeo helper).

// --- odysee -------------------------------------------------------------------

func odyseeID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	return strings.TrimPrefix(u.Path, "/"), nil
}

func odyseeData(ctx context.Context, f Fetcher, _ *Service, _, videoID string) (*VideoData, error) {
	body, err := getText(ctx, f, "https://odysee.com/"+videoID, nil)
	if err != nil {
		return nil, fmt.Errorf("odysee %s: %w", videoID, err)
	}
	url := reFind(`"contentUrl":(\s)?"([^"]+)"`, body, 2)
	if url == "" {
		return nil, fmt.Errorf("odysee %s: content url not found", videoID)
	}
	return &VideoData{URL: url}, nil
}

// --- appledeveloper -----------------------------------------------------------

func appleDeveloperID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	return reFind(`videos/play/([^/]+)/([\d]+)`, u.Path, 0), nil
}

func appleDeveloperData(ctx context.Context, f Fetcher, _ *Service, _, videoID string) (*VideoData, error) {
	body, err := getText(ctx, f, "https://developer.apple.com/"+videoID, nil)
	if err != nil {
		return nil, fmt.Errorf("appledeveloper %s: %w", videoID, err)
	}
	url := reFind(`https://devstreaming-cdn\.apple\.com/videos/([^.]+)/(cmaf\.m3u8)`, body, 0)
	if url == "" {
		return nil, fmt.Errorf("appledeveloper %s: content url not found", videoID)
	}
	return &VideoData{URL: url}, nil
}

// --- bitview ------------------------------------------------------------------

func bitviewID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	return u.Query().Get("v"), nil
}

func bitviewData(ctx context.Context, f Fetcher, svc *Service, _, videoID string) (*VideoData, error) {
	body, err := getText(ctx, f, svc.BaseURL+videoID, nil)
	if err != nil {
		return nil, fmt.Errorf("bitview %s: %w", videoID, err)
	}
	url := reFind(`src:\s?"([^"]+\.mp4)"`, body, 1)
	if url == "" {
		return nil, fmt.Errorf("bitview %s: video url not found", videoID)
	}
	return &VideoData{URL: url}, nil
}
