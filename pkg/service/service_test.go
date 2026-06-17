package service

import (
	"context"
	"errors"
	"net/url"
	"testing"
)

func TestGetService(t *testing.T) {
	cases := []struct {
		url  string
		host string // "" => no service
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "youtube"},
		{"https://youtu.be/dQw4w9WgXcQ", "youtube"},
		{"https://m.youtube.com/watch?v=abc", "youtube"},
		{"https://vk.com/video-1_2", "vk"},
		{"https://vkvideo.ru/video-1_2", "vk"},
		{"https://rutube.ru/video/abcdef/", "rutube"},
		{"https://vimeo.com/123456789", "vimeo"},
		{"https://player.vimeo.com/video/123", "vimeo"},
		{"https://www.twitch.tv/videos/123", "twitch"},
		{"https://x.com/user/status/123", "twitter"},
		{"https://ok.ru/video/12345", "okru"},
		{"https://yewtu.be/watch?v=abc", "invidious"},
		{"https://example.com/video.mp4", "custom"},
		{"http://localhost:8080/video.mp4", ""},
		{"https://unknown-service-xyz.example/watch", ""},
	}
	for _, c := range cases {
		svc := GetService(c.url)
		got := ""
		if svc != nil {
			got = svc.Host
		}
		if got != c.host {
			t.Errorf("GetService(%q) host = %q, want %q", c.url, got, c.host)
		}
	}
}

func TestVideoIDExtraction(t *testing.T) {
	cases := []struct {
		host string
		url  string
		want string
	}{
		{"youtube", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"youtube", "https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"youtube", "https://www.youtube.com/shorts/AbC123", "AbC123"},
		{"youtube", "https://www.youtube.com/embed/xyz", "xyz"},
		{"vk", "https://vk.com/video-1_2", "video-1_2"},
		{"vk", "https://vk.com/clip-5_6", "clip-5_6"},
		{"rutube", "https://rutube.ru/video/abcdef/", "abcdef"},
		{"okru", "https://ok.ru/video/987654", "987654"},
		{"twitter", "https://x.com/user/status/55555", "55555"},
		{"dailymotion", "https://dai.ly/xyz", "xyz"},
		{"dailymotion", "https://www.dailymotion.com/video/zzz", "zzz"},
		{"nine_gag", "https://9gag.com/gag/aZ1bC", "aZ1bC"},
		{"googledrive", "https://drive.google.com/file/d/FILEID/view", "FILEID"},
		{"vimeo", "https://vimeo.com/123456789", "123456789"},
	}
	for _, c := range cases {
		h, ok := helpers[c.host]
		if !ok || h.id == nil {
			t.Errorf("no id helper for %s", c.host)
			continue
		}
		u, _ := url.Parse(c.url)
		got, err := h.id(u)
		if err != nil {
			t.Errorf("%s id(%q) error: %v", c.host, c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s id(%q) = %q, want %q", c.host, c.url, got, c.want)
		}
	}
}

func TestGetVideoDataSimple(t *testing.T) {
	// A simple (no extra data) service builds url = baseURL + videoId.
	vd, err := GetVideoData(context.Background(), nil, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil {
		t.Fatal(err)
	}
	if vd.Host != "youtube" || vd.VideoID != "dQw4w9WgXcQ" || vd.URL != "https://youtu.be/dQw4w9WgXcQ" {
		t.Fatalf("got %+v", vd)
	}
}

func TestGetVideoDataCustom(t *testing.T) {
	const link = "https://cdn.example/movie.mp4"
	vd, err := GetVideoData(context.Background(), nil, link)
	if err != nil {
		t.Fatal(err)
	}
	if vd.Host != "custom" || vd.URL != link {
		t.Fatalf("got %+v", vd)
	}
}

func TestGetVideoDataUnknown(t *testing.T) {
	_, err := GetVideoData(context.Background(), nil, "https://unknown-xyz.example/watch")
	if !errors.Is(err, ErrUnknownService) {
		t.Fatalf("err = %v, want ErrUnknownService", err)
	}
}

func TestGetVideoDataNotImplemented(t *testing.T) {
	// kodik is registered but needs extra data with no ported helper.
	_, err := GetVideoData(context.Background(), nil, "https://kodik.info/seria/123/abc/720p")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", err)
	}
}
