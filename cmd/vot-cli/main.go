// Command vot-cli downloads Yandex voice-over translations (and subtitles) for
// videos from the command line. It mirrors the FOSWLY vot-cli UX.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/n0madic/go-vot/pkg/client"
	"github.com/n0madic/go-vot/pkg/config"
	"github.com/n0madic/go-vot/pkg/lang"
	"github.com/n0madic/go-vot/pkg/service"
	"github.com/n0madic/go-vot/pkg/subs"
	"github.com/n0madic/go-vot/pkg/vot"
)

// videoMux is a flag.Value with an optional argument. Bare --video-mux enables
// muxing (Set is called with "true"); --video-mux=<file|url> sets the source.
type videoMux struct {
	set    bool
	source string
}

func (m *videoMux) String() string { return m.source }

func (m *videoMux) Set(s string) error {
	m.set = true
	if s != "true" { // the bare form passes the literal "true"
		m.source = s
	}
	return nil
}

// IsBoolFlag lets the flag be used without a value (bare --video-mux).
func (m *videoMux) IsBoolFlag() bool { return true }

func clientSubtitlesParams(vd *service.VideoData, requestLang string) client.SubtitlesParams {
	return client.SubtitlesParams{
		URL:         vd.URL,
		VideoID:     vd.VideoID,
		Host:        vd.Host,
		RequestLang: requestLang,
	}
}

const version = "0.1.0"

func main() {
	var (
		flagLang       = flag.String("lang", "auto", "source video language")
		flagResLang    = flag.String("reslang", "ru", "target (TTS) language: ru, en or kk")
		flagOutput     = flag.String("output", ".", "directory to save files into")
		flagOutputFile = flag.String("output-file", "", "output filename (requires --output)")
		flagSubs       = flag.Bool("subs", false, "download subtitles instead of audio")
		flagSubsSrt    = flag.Bool("subs-srt", false, "save subtitles as .srt (default: .vtt)")
		flagProxy      = flag.String("proxy", "", "HTTP/HTTPS proxy URL")
		flagWorker     = flag.Bool("worker", false, "route requests through the VOT worker proxy (geo bypass)")
		flagURLOnly    = flag.Bool("url-only", false, "print the result URL without downloading")
		flagVersion    = flag.Bool("version", false, "print version and exit")
	)

	// --video-mux takes an optional value: bare --video-mux enables muxing with
	// the auto-resolved media URL (direct links), --video-mux=<file|url> sets the
	// source explicitly.
	var muxFlag videoMux
	flag.Var(&muxFlag, "video-mux", "mux translated audio into the source video via ffmpeg; bare --video-mux fetches the source with yt-dlp (or the media URL for direct links), --video-mux=<file|url> sets it explicitly")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "vot-cli %s — Yandex voice-over translation downloader\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n  vot-cli [options] <link> [link2 ...]\n\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  vot-cli --reslang=ru https://youtu.be/dQw4w9WgXcQ\n")
		fmt.Fprintf(os.Stderr, "  vot-cli --subs --subs-srt --output=. https://youtu.be/ID\n")
		fmt.Fprintf(os.Stderr, "  vot-cli --video-mux=source.mp4 https://youtu.be/ID\n")
		fmt.Fprintf(os.Stderr, "  vot-cli --video-mux \"https://example.com/clip.mp4\"\n")
	}
	flag.Parse()

	if *flagVersion {
		fmt.Printf("vot-cli %s (vot.js %s)\n", version, config.LibVersion)
		return
	}

	links := flag.Args()
	if len(links) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	wantSubs := *flagSubs
	subsSrt := *flagSubsSrt

	if !lang.IsAvailableTTS(*flagResLang) {
		fmt.Fprintf(os.Stderr, "warning: target language %q is not in the known TTS list %v; trying anyway\n", *flagResLang, lang.AvailableTTS)
	}

	httpClient, err := buildHTTPClient(*flagProxy)
	if err != nil {
		fatal(err)
	}

	v, err := vot.New(vot.Options{
		RequestLang:  *flagLang,
		ResponseLang: *flagResLang,
		Proxy:        *flagProxy,
		Worker:       *flagWorker,
	})
	if err != nil {
		fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app := &app{
		v:          v,
		http:       httpClient,
		lang:       *flagLang,
		resLang:    *flagResLang,
		output:     *flagOutput,
		outputFile: *flagOutputFile,
		subsSrt:    subsSrt,
		mux:        muxFlag.set,
		video:      muxFlag.source,
		urlOnly:    *flagURLOnly,
	}

	exitCode := 0
	for _, link := range links {
		var err error
		if wantSubs {
			err = app.processSubtitles(ctx, link)
		} else {
			err = app.processAudio(ctx, link)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s: %v\n", link, err)
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

type app struct {
	v          *vot.Client
	http       *http.Client
	lang       string
	resLang    string
	output     string
	outputFile string
	subsSrt    bool
	mux        bool
	video      string
	urlOnly    bool
}

func (a *app) processAudio(ctx context.Context, link string) error {
	fmt.Printf("→ %s\n", link)

	res, err := a.v.Translate(ctx, link, vot.TranslateOptions{
		RequestLang:  a.lang,
		ResponseLang: a.resLang,
		OnProgress: func(status string, remaining int32) {
			if remaining > 0 {
				fmt.Printf("  %s, waiting ~%ds...\n", status, remaining)
			} else {
				fmt.Printf("  %s...\n", status)
			}
		},
	})
	if err != nil {
		return err
	}

	fmt.Printf("  audio: %s\n", res.URL)
	if a.urlOnly {
		return nil
	}

	if err := os.MkdirAll(a.output, 0o755); err != nil {
		return err
	}
	name := a.baseName(res.VideoData.Title, res.VideoData.VideoID)
	dest := filepath.Join(a.output, fileNameOr(a.outputFile, name+".mp3"))

	n, err := downloadFile(ctx, a.http, res.URL, dest)
	if err != nil {
		return err
	}
	fmt.Printf("  saved %s (%s)\n", dest, humanSize(n))

	if a.mux {
		videoSource := a.video
		if videoSource == "" {
			switch {
			case res.VideoData.Host == "custom":
				videoSource = res.VideoData.URL
			case ytdlpAvailable():
				fmt.Println("  downloading source video with yt-dlp...")
				p, err := ytdlpDownload(ctx, link, a.output, name+".source")
				if err != nil {
					return err
				}
				videoSource = p
				fmt.Printf("  source video: %s\n", videoSource)
			default:
				return errors.New("--video-mux needs a <file|url> value for non-direct services (or install yt-dlp to fetch the source automatically)")
			}
		}
		out := filepath.Join(a.output, name+".mux.mp4")
		fmt.Printf("  muxing into %s...\n", out)
		if err := muxAudio(ctx, videoSource, dest, out, lang.ISO6392(a.resLang)); err != nil {
			return err
		}
		fmt.Printf("  muxed %s\n", out)
	}
	return nil
}

func (a *app) processSubtitles(ctx context.Context, link string) error {
	fmt.Printf("→ %s\n", link)

	vd, err := a.v.GetVideoData(ctx, link)
	if err != nil {
		return err
	}
	result, err := a.v.Raw().GetSubtitles(ctx, clientSubtitlesParams(vd, a.lang))
	if err != nil {
		return err
	}
	if len(result.Subtitles) == 0 {
		if result.Waiting {
			return errors.New("subtitles are still being generated, try again later")
		}
		return errors.New("no subtitles available")
	}

	// Prefer a track translated into the target language, else the first one.
	track := result.Subtitles[0]
	for _, s := range result.Subtitles {
		if s.TranslatedLanguage == a.resLang && s.TranslatedURL != "" {
			track = s
			break
		}
	}
	srcURL := track.TranslatedURL
	srcLang := track.TranslatedLanguage
	if srcURL == "" {
		srcURL = track.URL
		srcLang = track.Language
	}

	raw, err := fetchBytes(ctx, a.http, srcURL)
	if err != nil {
		return err
	}
	format := "vtt"
	if a.subsSrt {
		format = "srt"
	}
	converted, err := subs.Convert(raw, format)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(a.output, 0o755); err != nil {
		return err
	}
	name := a.baseName(vd.Title, vd.VideoID)
	dest := filepath.Join(a.output, fileNameOr(a.outputFile, fmt.Sprintf("%s.%s.%s", name, srcLang, format)))
	if err := os.WriteFile(dest, converted, 0o644); err != nil {
		return err
	}
	fmt.Printf("  saved subtitles %s\n", dest)
	return nil
}

func (a *app) baseName(title, fallback string) string {
	if title != "" {
		return sanitize(title)
	}
	return sanitize(fallback)
}

var unsafeChars = regexp.MustCompile(`[^\w.\-]+`)

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = unsafeChars.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_.")
	if s == "" {
		return "video"
	}
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}

func fileNameOr(name, def string) string {
	if name != "" {
		return name
	}
	return def
}

func buildHTTPClient(proxy string) (*http.Client, error) {
	tr := &http.Transport{}
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy: %w", err)
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Timeout: 5 * time.Minute, Transport: tr}, nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
