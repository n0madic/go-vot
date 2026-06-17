package service

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// fakeDoer routes requests to a per-test handler, satisfying Fetcher.
type fakeDoer struct {
	fn func(*http.Request) (*http.Response, error)
}

func (f fakeDoer) Do(r *http.Request) (*http.Response, error) { return f.fn(r) }

func textResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// TestPeertubeAndCloudflareResolveOffline verifies the id-only (Tier 0) services
// build "origin + id" without any HTTP call.
func TestPeertubeAndCloudflareResolveOffline(t *testing.T) {
	cases := []struct {
		url     string
		host    string
		wantURL string
	}{
		{"https://makertube.net/w/abc123", HostPeertube, "https://makertube.net/w/abc123"},
		{"https://watch.cloudflarestream.com/vid42?x=1", HostCloudflareStream, "https://watch.cloudflarestream.com/vid42?x=1"},
	}
	for _, c := range cases {
		vd, err := GetVideoData(context.Background(), nil, c.url)
		if err != nil {
			t.Errorf("GetVideoData(%q): %v", c.url, err)
			continue
		}
		if vd.Host != c.host || vd.URL != c.wantURL {
			t.Errorf("GetVideoData(%q) = host %q url %q, want host %q url %q", c.url, vd.Host, vd.URL, c.host, c.wantURL)
		}
	}
}

func TestBannedVideoData(t *testing.T) {
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.banned.video" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL)
		}
		return textResp(200, `{"data":{"getVideo":{"title":"Banned Title","videoUrl":"https://api.banned.video/v.mp4","duration":123}}}`), nil
	}}
	vd, err := GetVideoData(context.Background(), doer, "https://madmaxworld.tv/watch?id=vid1")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://api.banned.video/v.mp4" || vd.Title != "Banned Title" || vd.Duration != 123 {
		t.Fatalf("got %+v", vd)
	}
}

func TestOdyseeData(t *testing.T) {
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		return textResp(200, `<html><head><script>{"contentUrl": "https://player.odycdn.com/v/x.mp4"}</script></head></html>`), nil
	}}
	vd, err := GetVideoData(context.Background(), doer, "https://odysee.com/@chan:1/video:2")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://player.odycdn.com/v/x.mp4" {
		t.Fatalf("got %+v", vd)
	}
}

func TestEpicGamesData(t *testing.T) {
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(r.URL.Path, "/post.json"):
			return textResp(200, `{"title":"Epic Course","blocks":[{"type":"text"},{"type":"video","video_id":"emb42"}]}`), nil
		case strings.Contains(r.URL.Path, "/embed.html"):
			return textResp(200, `<script>var videoUrl = "qsep://video.epicgames.com/playlist.m3u8";</script>`), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, nil
	}}
	vd, err := GetVideoData(context.Background(), doer, "https://dev.epicgames.com/community/learning/tutorials/1pV5/title")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://video.epicgames.com/playlist.m3u8" || vd.Title != "Epic Course" {
		t.Fatalf("got %+v", vd)
	}
}

func TestIgnDataByNext(t *testing.T) {
	html := `<html><body><script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"page":{"title":"IGN Vid","description":"d","video":{"videoMetadata":{"duration":88},"assets":[{"url":"https://ign.cdn/low.mp4","height":360},{"url":"https://ign.cdn/high.mp4","height":1080}]}}}}}` +
		`</script></body></html>`
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		return textResp(200, html), nil
	}}
	vd, err := GetVideoData(context.Background(), doer, "https://www.ign.com/de/1/video/x")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://ign.cdn/low.mp4" || vd.Title != "IGN Vid" || vd.Duration != 88 {
		t.Fatalf("got %+v", vd)
	}
}

func TestIgnDataByScriptData(t *testing.T) {
	html := `<html><head>` +
		`<script type="application/ld+json">{"@type":"WebPage"}</script>` +
		`<script type="application/ld+json">{"@type":"VideoObject","name":"LD Vid","contentUrl":"https://ign.cdn/ld.mp4","description":"x"}</script>` +
		`</head></html>`
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		return textResp(200, html), nil
	}}
	vd, err := GetVideoData(context.Background(), doer, "https://www.ign.com/videos/some-slug")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://ign.cdn/ld.mp4" || vd.Title != "LD Vid" {
		t.Fatalf("got %+v", vd)
	}
}

// TestIgnDataByIcms covers the ICMS branch where the container carries extra
// classes around the icmsvideocontainer token (the case the prior exact-quote
// regex missed).
func TestIgnDataByIcms(t *testing.T) {
	html := `<html><body>` +
		`<div class="player-wrapper icmsvideocontainer responsive" ` +
		`data-json="{&quot;title&quot;:&quot;ICMS Vid&quot;,&quot;mediaFiles&quot;:{&quot;360&quot;:&quot;https://ign.cdn/icms360.mp4&quot;}}"></div>` +
		`</body></html>`
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		return textResp(200, html), nil
	}}
	vd, err := GetVideoData(context.Background(), doer, "https://www.ign.com/videos/icms-slug")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://ign.cdn/icms360.mp4" || vd.Title != "ICMS Vid" {
		t.Fatalf("got %+v", vd)
	}
}

func TestYandexDiskSingleFileOffline(t *testing.T) {
	// A direct /i/ or single /d/<hash> link resolves to the public viewer URL
	// with no HTTP call.
	vd, err := GetVideoData(context.Background(), nil, "https://disk.yandex.ru/i/abc123")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://yadi.sk/i/abc123" {
		t.Fatalf("got %+v", vd)
	}
}

func TestYandexDiskFolderData(t *testing.T) {
	prefetch := `{"rootResourceId":"root1","environment":{"sk":"sk1"},"resources":{` +
		`"root1":{"name":"folder","type":"dir","hash":"H"},` +
		`"r2":{"name":"movie.mp4","type":"file","path":"/disk/movie.mp4","meta":{"short_url":"https://yadi.sk/i/short","mediatype":"video","videoDuration":120000}}}}`
	html := `<html><body><script type="application/json" id="store-prefetch">` + prefetch + `</script></body></html>`
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.Path, "/d/HASH/movie.mp4") {
			t.Fatalf("unexpected request %s", r.URL)
		}
		return textResp(200, html), nil
	}}
	vd, err := GetVideoData(context.Background(), doer, "https://disk.yandex.ru/d/HASH/movie.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://yadi.sk/i/short" || vd.Title != "movie" || vd.Duration != 120 {
		t.Fatalf("got %+v", vd)
	}
}

// TestYandexDiskFolderFetchList exercises the deep-folder branch: a path with a
// nested resourcePath (>=2 intermediate segments) triggers the fetch-list API,
// and a resource without short_url triggers the download-url API.
func TestYandexDiskFolderFetchList(t *testing.T) {
	prefetch := `{"rootResourceId":"root1","environment":{"sk":"sk1"},"resources":{` +
		`"root1":{"name":"folder","type":"dir","hash":"ROOTHASH"}}}`
	page := `<html><body><script type="application/json" id="store-prefetch">` + prefetch + `</script></body></html>`
	var fetchListBody, downloadBody string
	doer := fakeDoer{fn: func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/d/HASH/a/b/movie.mp4"):
			return textResp(200, page), nil
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/public/api/fetch-list"):
			b, _ := io.ReadAll(r.Body)
			fetchListBody = string(b)
			return textResp(200, `{"resources":[{"name":"movie.mp4","type":"file","path":"/disk/a/b/movie.mp4","meta":{"mediatype":"video","videoDuration":90000}}]}`), nil
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/public/api/download-url"):
			b, _ := io.ReadAll(r.Body)
			downloadBody = string(b)
			return textResp(200, `{"data":{"url":"https://downloader.disk.yandex.net/x.mp4"}}`), nil
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL)
		return nil, nil
	}}
	vd, err := GetVideoData(context.Background(), doer, "https://disk.yandex.ru/d/HASH/a/b/movie.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if vd.URL != "https://downloader.disk.yandex.net/x.mp4" || vd.Title != "movie" || vd.Duration != 90 {
		t.Fatalf("got %+v", vd)
	}
	// The folder hash is built from the root resource hash and the intermediate path.
	if want := url.QueryEscape(`{"hash":"ROOTHASH:/a/b","sk":"sk1"}`); fetchListBody != want {
		t.Errorf("fetch-list body = %q, want %q", fetchListBody, want)
	}
	if want := url.QueryEscape(`{"hash":"/disk/a/b/movie.mp4","sk":"sk1"}`); downloadBody != want {
		t.Errorf("download-url body = %q, want %q", downloadBody, want)
	}
}
