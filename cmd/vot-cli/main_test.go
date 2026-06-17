package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/n0madic/go-vot/pkg/client"
	"github.com/n0madic/go-vot/pkg/service"
)

func TestSanitize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Hello World", "Hello_World"},
		{"  trim  me  ", "trim_me"},
		{`a/b\c:d*e?f"g<h>i|j`, "abcdefghij"},
		{"Привет, мир!", "Привет,_мир!"},
		{"日本語テスト", "日本語テスト"},
		{"", "video"},
		{"///", "video"},
		{"...___...", "video"},
		// Control chars (tab, newline) are in the unsafe \x00-\x1f range and are
		// stripped outright; only regular spaces collapse to underscores.
		{"tab\tand\nnewline", "tabandnewline"},
	}
	for _, c := range cases {
		if got := sanitize(c.in); got != c.want {
			t.Errorf("sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeRuneCap(t *testing.T) {
	// 200 Cyrillic runes (multibyte) must be capped to 120 runes, not split.
	long := make([]rune, 200)
	for i := range long {
		long[i] = 'я'
	}
	got := sanitize(string(long))
	if r := []rune(got); len(r) != 120 {
		t.Errorf("sanitize cap = %d runes, want 120", len(r))
	}
}

func TestYtdlpSourceURL(t *testing.T) {
	cases := []struct {
		name     string
		vd       *service.VideoData
		fallback string
		want     string
	}{
		{
			name:     "youtube rebuilds canonical watch url",
			vd:       &service.VideoData{Host: "youtube", VideoID: "dQw4w9WgXcQ"},
			fallback: "https://youtu.be/dQw4w9WgXcQ?list=PL&index=2",
			want:     "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:     "invidious is youtube family",
			vd:       &service.VideoData{Host: "invidious", VideoID: "abc"},
			fallback: "https://yewtu.be/watch?v=abc",
			want:     "https://www.youtube.com/watch?v=abc",
		},
		{
			name:     "non-youtube uses the original link",
			vd:       &service.VideoData{Host: "vimeo", VideoID: "123"},
			fallback: "https://vimeo.com/123",
			want:     "https://vimeo.com/123",
		},
		{
			name:     "youtube without id falls back",
			vd:       &service.VideoData{Host: "youtube", VideoID: ""},
			fallback: "https://youtube.com/weird",
			want:     "https://youtube.com/weird",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ytdlpSourceURL(c.vd, c.fallback); got != c.want {
				t.Errorf("ytdlpSourceURL = %q, want %q", got, c.want)
			}
		})
	}
}

func TestIsYouTubeFamily(t *testing.T) {
	yes := []string{"youtube", "invidious", "piped", "poketube", "ricktube"}
	for _, h := range yes {
		if !isYouTubeFamily(h) {
			t.Errorf("isYouTubeFamily(%q) = false, want true", h)
		}
	}
	no := []string{"vimeo", "vk", "twitch", "", "youtubekids"}
	for _, h := range no {
		if isYouTubeFamily(h) {
			t.Errorf("isYouTubeFamily(%q) = true, want false", h)
		}
	}
}

func TestReadLinksFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "links.txt")
	content := "# a comment\n" +
		"https://youtu.be/one\n" +
		"\n" +
		"   \n" +
		"  https://youtu.be/two  \n" +
		"# another comment\n" +
		"https://youtu.be/three\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	links, err := readLinksFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://youtu.be/one", "https://youtu.be/two", "https://youtu.be/three"}
	if len(links) != len(want) {
		t.Fatalf("got %v, want %v", links, want)
	}
	for i := range want {
		if links[i] != want[i] {
			t.Errorf("link[%d] = %q, want %q", i, links[i], want[i])
		}
	}

	if _, err := readLinksFile(filepath.Join(dir, "nope.txt")); err == nil {
		t.Error("readLinksFile(missing) = nil error, want error")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"", "", "x"}, "x"},
		{[]string{"a", "b"}, "a"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := firstNonEmpty(c.in...); got != c.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestPickSubtitleTrack(t *testing.T) {
	cases := []struct {
		name    string
		tracks  []client.SubtitleTrack
		resLang string
		wantURL string
		wantLng string
	}{
		{
			name: "prefers reslang translation",
			tracks: []client.SubtitleTrack{
				{Language: "en", URL: "en.vtt", TranslatedLanguage: "kk", TranslatedURL: "kk.vtt"},
				{Language: "en", URL: "en.vtt", TranslatedLanguage: "ru", TranslatedURL: "ru.vtt"},
			},
			resLang: "ru",
			wantURL: "ru.vtt",
			wantLng: "ru",
		},
		{
			name: "falls back to first track translation",
			tracks: []client.SubtitleTrack{
				{Language: "en", URL: "en.vtt", TranslatedLanguage: "de", TranslatedURL: "de.vtt"},
			},
			resLang: "ru",
			wantURL: "de.vtt",
			wantLng: "de",
		},
		{
			name: "falls back to original when no translation",
			tracks: []client.SubtitleTrack{
				{Language: "en", URL: "en.vtt"},
			},
			resLang: "ru",
			wantURL: "en.vtt",
			wantLng: "en",
		},
		{
			name:    "empty tracks",
			tracks:  nil,
			resLang: "ru",
			wantURL: "",
			wantLng: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotURL, gotLng := pickSubtitleTrack(c.tracks, c.resLang)
			if gotURL != c.wantURL || gotLng != c.wantLng {
				t.Errorf("pickSubtitleTrack = (%q, %q), want (%q, %q)", gotURL, gotLng, c.wantURL, c.wantLng)
			}
		})
	}
}

func TestParseVideoQuality(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"best", 0, false},
		{"", 0, false},
		{"1080", 1080, false},
		{"2160", 2160, false},
		{"480", 480, false},
		{"abc", 0, true},
		{"-5", 0, true},
		{"0", 0, true},
	}
	for _, c := range cases {
		got, err := parseVideoQuality(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseVideoQuality(%q) = nil error, want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseVideoQuality(%q) unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseVideoQuality(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestYtdlpFormat(t *testing.T) {
	if got := ytdlpFormat(0); got != "bv*+ba/b" {
		t.Errorf("ytdlpFormat(0) = %q, want best selector", got)
	}
	want := "bv*[height<=1080]+ba/b[height<=1080]/bv*+ba/b"
	if got := ytdlpFormat(1080); got != want {
		t.Errorf("ytdlpFormat(1080) = %q, want %q", got, want)
	}
}
