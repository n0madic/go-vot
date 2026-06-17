package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// helper bundles a video-id extractor and an optional extra-data fetcher for a
// service. Add an entry to the helpers map to support a new service.
type helper struct {
	id   func(ctx context.Context, f Fetcher, u *url.URL) (string, error)
	data func(ctx context.Context, f Fetcher, svc *Service, origin, videoID string) (*VideoData, error)
}

// reFind returns the submatch at index from the first match of pattern on s, or "".
func reFind(pattern, s string, index int) string {
	m := regexp.MustCompile(pattern).FindStringSubmatch(s)
	if m == nil || index >= len(m) {
		return ""
	}
	return m[index]
}

// --- id extractors (ported from @vot.js/node/helpers/*.js) --------------------

func ytID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	if u.Hostname() == "youtu.be" {
		return strings.TrimPrefix(u.Path, "/"), nil
	}
	if id := reFind(`(?:watch|embed|shorts|live)/([^/]+)`, u.Path, 1); id != "" {
		return id, nil
	}
	return u.Query().Get("v"), nil
}

func vkID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	if m := reFind(`^/(video|clip)-?\d+_\d+$`, u.Path, 0); m != "" {
		return strings.TrimPrefix(m, "/"), nil
	}
	if id := reFind(`/playlist/[^/]+/(video-?\d+_\d+)`, u.Path, 1); id != "" {
		return id, nil
	}
	if z := u.Query().Get("z"); z != "" {
		return strings.SplitN(z, "/", 2)[0], nil
	}
	oid := u.Query().Get("oid")
	id := u.Query().Get("id")
	if oid != "" && id != "" {
		if n, err := strconv.Atoi(oid); err == nil {
			if n < 0 {
				n = -n
			}
			return "video-" + strconv.Itoa(n) + "_" + id, nil
		}
	}
	return "", nil
}

func twitchID(ctx context.Context, f Fetcher, u *url.URL) (string, error) {
	if regexp.MustCompile(`^player\.twitch\.tv$`).MatchString(u.Hostname()) {
		return "videos/" + u.Query().Get("video"), nil
	}
	if clip := reFind(`([^/]+)/(?:clip)/([^/]+)`, u.Path, 0); clip != "" {
		return clip, nil
	}
	if u.Hostname() == "clips.twitch.tv" {
		return twitchClipLink(ctx, f, u)
	}
	return reFind(`(?:videos)/([^/]+)`, u.Path, 0), nil
}

func tiktokID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	return reFind(`([^/]+)/video/([^/]+)`, u.Path, 0), nil
}

func vimeoID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	embedID := reFind(`video/[^/]+$`, u.Path, 0)
	if embedID != "" {
		videoID := strings.TrimPrefix(embedID, "video/")
		if hash := u.Query().Get("h"); hash != "" {
			return videoID + "/" + hash, nil
		}
		return videoID, nil
	}
	if id := reFind(`channels/[^/]+/([^/]+)`, u.Path, 1); id != "" {
		return id, nil
	}
	if id := reFind(`groups/[^/]+/videos/([^/]+)`, u.Path, 1); id != "" {
		return id, nil
	}
	if id := reFind(`(showcase|album)/[^/]+/video/([^/]+)`, u.Path, 2); id != "" {
		return id, nil
	}
	return strings.TrimPrefix(u.Path, "/"), nil
}

func mailruID(ctx context.Context, f Fetcher, u *url.URL) (string, error) {
	if regexp.MustCompile(`/(v|mail|bk|inbox)/`).MatchString(u.Path) {
		return strings.TrimPrefix(u.Path, "/"), nil
	}
	videoID := reFind(`video/embed/([^/]+)`, u.Path, 1)
	if videoID == "" {
		return "", nil
	}
	var meta struct {
		Meta struct {
			URL string `json:"url"`
		} `json:"meta"`
	}
	metaURL := "https://my.mail.ru/+/video/meta/" + videoID + "?ajax_call=1&ext=1"
	if err := getJSON(ctx, f, metaURL, nil, &meta); err != nil {
		return "", err
	}
	if meta.Meta.URL == "" {
		return "", fmt.Errorf("empty meta url for mailru embed %s", videoID)
	}
	return strings.TrimPrefix(meta.Meta.URL, "//my.mail.ru/"), nil
}

func bilibiliID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	if b := reFind(`bangumi/play/([^/]+)`, u.Path, 0); b != "" {
		return b, nil
	}
	if bvid := u.Query().Get("bvid"); bvid != "" {
		return "video/" + bvid, nil
	}
	vid := reFind(`video/([^/]+)`, u.Path, 0)
	if vid != "" && u.Query().Has("p") {
		vid += "/?p=" + u.Query().Get("p")
	}
	return vid, nil
}

func trovoID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	vid := u.Query().Get("vid")
	path := reFind(`([^/]+)/([\d]+)`, u.Path, 0)
	if vid == "" || path == "" {
		return "", nil
	}
	return path + "?vid=" + vid, nil
}

// pathSlice returns the path without its leading slash.
func pathSlice(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	return strings.TrimPrefix(u.Path, "/"), nil
}

// helpers maps a service host to its extractor(s). Only services with an entry
// here can resolve a video id; the rest return ErrNotImplemented.
var helpers = map[string]helper{
	HostYouTube: {id: ytID},
	"invidious": {id: ytID},
	"piped":     {id: ytID},
	"poketube":  {id: ytID},
	"ricktube":  {id: ytID},
	"vk":        {id: vkID},
	"nine_gag": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`gag/([^/]+)`, u.Path, 1), nil
	}},
	"twitch":   {id: twitchID},
	"proxitok": {id: tiktokID},
	"tiktok":   {id: tiktokID},
	HostVimeo:  {id: vimeoID, data: vimeoData},
	"xvideos": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`[^/]+/[^/]+$`, u.Path, 0), nil
	}},
	"pornhub": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		if vk := u.Query().Get("viewkey"); vk != "" {
			return vk, nil
		}
		return reFind(`embed/([^/]+)`, u.Path, 1), nil
	}},
	"twitter": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`status/([^/]+)`, u.Path, 1), nil
	}},
	"rumble":   {id: pathSlice},
	"facebook": {id: pathSlice},
	"rutube": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`(?:video|embed)/([^/]+)`, u.Path, 1), nil
	}},
	"bilibili": {id: bilibiliID},
	"mailru":   {id: mailruID},
	"bitchute": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`(video|embed)/([^/]+)`, u.Path, 2), nil
	}},
	"eporner": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`video-([^/]+)/([^/]+)`, u.Path, 0), nil
	}},
	"dailymotion": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		if u.Hostname() == "dai.ly" {
			return strings.TrimPrefix(u.Path, "/"), nil
		}
		return reFind(`video/([^/]+)`, u.Path, 1), nil
	}},
	"trovo": {id: trovoID},
	"okru": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`/video/(\d+)`, u.Path, 1), nil
	}},
	"googledrive": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`/file/d/([^/]+)`, u.Path, 1), nil
	}},
	"newgrounds": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`([^/]+)/(view)/([^/]+)`, u.Path, 0), nil
	}},
	"egghead": {id: pathSlice},
	"youku": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`v_show/id_[\w=]+`, u.Path, 0), nil
	}},
	"archive": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`(details|embed)/([^/]+)`, u.Path, 2), nil
	}},
	"watchpornto": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`(video|embed)/(\d+)(/[^/]+/)?`, u.Path, 0), nil
	}},
	"dzen": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`video/watch/([^/]+)`, u.Path, 1), nil
	}},
	"loom": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`(embed|share)/([^/]+)?`, u.Path, 2), nil
	}},
	"imdb": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`video/([^/]+)`, u.Path, 1), nil
	}},
	"thisvid": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`(videos|embed)/[^/]+`, u.Path, 0), nil
	}},
	"telegram": {id: func(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
		return reFind(`([^/]+)/(\d+)`, u.Path, 0), nil
	}},
	"kick": {id: kickID, data: kickData},
}
