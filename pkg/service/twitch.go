package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// twitchClipLink resolves the channel/clip path for a clips.twitch.tv URL by
// fetching the clip page and extracting the channel name from the embedded JSON.
// Ported from @vot.js/node/helpers/twitch.js getClipLink.
func twitchClipLink(ctx context.Context, f Fetcher, u *url.URL) (string, error) {
	clearPathname := strings.TrimPrefix(u.Path, "/")
	var videoPath string
	if clearPathname == "embed" {
		videoPath = u.Query().Get("clip")
	} else {
		videoPath = clearPathname
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://clips.twitch.tv/"+videoPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Googlebot/2.1 (+http://www.googlebot.com/bot.html)")

	resp, err := f.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitch clip: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	m := regexp.MustCompile(`"url":"https://www\.twitch\.tv/([^"]+)"`).FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("twitch clip: channel link not found in page")
	}
	return string(m[1]) + "/clip/" + videoPath, nil
}
