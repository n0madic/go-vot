package service

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

func ignID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	if id := reFind(`([^/]+)/([\d]+)/video/([^/]+)`, u.Path, 0); id != "" {
		return id, nil
	}
	return reFind(`/videos/([^/]+)`, u.Path, 0), nil
}

// ignData resolves an IGN video page. IGN serves video metadata in one of three
// shapes (Next.js data, an ICMS container, or ld+json); it tries them in the
// same order as upstream and falls back to the base page URL on any failure.
// Ported from @vot.js/node/helpers/ign.js. The raw media URL is returned as-is
// (upstream proxies it via media-proxy; not ported, as in the vimeo helper).
func ignData(ctx context.Context, f Fetcher, svc *Service, _, videoID string) (*VideoData, error) {
	base := &VideoData{URL: svc.BaseURL + videoID}

	body, err := getText(ctx, f, svc.BaseURL+videoID, nil)
	if err != nil {
		return base, nil
	}

	var vd *VideoData
	switch {
	case strings.Contains(body, "__NEXT_DATA__"):
		vd = ignByNext(body)
	case strings.Contains(body, "icmsvideocontainer"):
		vd = ignByIcms(body)
	default:
		vd = ignByScriptData(body)
	}
	if vd == nil {
		return base, nil
	}
	return vd, nil
}

func ignByNext(body string) *VideoData {
	raw := reFind(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`, body, 1)
	if raw == "" {
		return nil
	}
	var data struct {
		Props struct {
			PageProps struct {
				Page struct {
					Title       string `json:"title"`
					Description string `json:"description"`
					Video       struct {
						VideoMetadata struct {
							Duration float64 `json:"duration"`
						} `json:"videoMetadata"`
						Assets []struct {
							URL    string `json:"url"`
							Height int    `json:"height"`
						} `json:"assets"`
					} `json:"video"`
				} `json:"page"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil
	}
	page := data.Props.PageProps.Page
	for _, a := range page.Video.Assets {
		if a.Height == 360 && strings.Contains(a.URL, ".mp4") {
			return &VideoData{URL: a.URL, Title: page.Title, Duration: page.Video.VideoMetadata.Duration}
		}
	}
	return nil
}

func ignByIcms(body string) *VideoData {
	raw := reFind(`icmsvideocontainer"[^>]*\bdata-json="([^"]*)"`, body, 1)
	if raw == "" {
		return nil
	}
	var data struct {
		Title      string            `json:"title"`
		MediaFiles map[string]string `json:"mediaFiles"`
		IsLive     bool              `json:"is_live"`
	}
	if err := json.Unmarshal([]byte(strings.ReplaceAll(raw, "&quot;", `"`)), &data); err != nil {
		return nil
	}
	url := data.MediaFiles["360"]
	if url == "" {
		return nil
	}
	return &VideoData{URL: url, Title: data.Title}
}

func ignByScriptData(body string) *VideoData {
	re := regexp.MustCompile(`(?s)<script type="application/ld\+json"[^>]*>(.*?)</script>`)
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		if !strings.Contains(m[1], "contentUrl") {
			continue
		}
		var data struct {
			ContentURL  string `json:"contentUrl"`
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
			continue
		}
		if data.ContentURL != "" {
			return &VideoData{URL: data.ContentURL, Title: data.Name}
		}
	}
	return nil
}
