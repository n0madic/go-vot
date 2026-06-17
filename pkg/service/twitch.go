package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// twitchClipLink resolves the channel/clip path for a clips.twitch.tv URL by
// fetching the clip page and extracting the channel name from the embedded JSON.
// Ported from @vot.js/node/helpers/twitch.js getClipLink.
func twitchClipLink(ctx context.Context, f Fetcher, u *url.URL) (string, error) {
	clearPathname := strings.TrimPrefix(u.Path, "/")
	videoPath := clearPathname
	if clearPathname == "embed" {
		videoPath = u.Query().Get("clip")
	}

	body, err := getText(ctx, f, "https://clips.twitch.tv/"+videoPath, map[string]string{
		"User-Agent": "Googlebot/2.1 (+http://www.googlebot.com/bot.html)",
	})
	if err != nil {
		return "", err
	}

	channel := reFind(`"url":"https://www\.twitch\.tv/([^"]+)"`, body, 1)
	if channel == "" {
		return "", fmt.Errorf("twitch clip: channel link not found in page")
	}
	return channel + "/clip/" + videoPath, nil
}
