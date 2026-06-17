// Command vot-cli downloads Yandex voice-over translations (and subtitles) for
// videos from the command line. It mirrors the FOSWLY vot-cli UX.
package main

import (
	"bufio"
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
	"strconv"
	"strings"
	"time"

	"github.com/n0madic/go-vot/pkg/client"
	"github.com/n0madic/go-vot/pkg/config"
	"github.com/n0madic/go-vot/pkg/lang"
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

const version = "0.1.0"

func main() {
	var (
		flagLang         = flag.String("lang", "auto", "source video language")
		flagResLang      = flag.String("reslang", "ru", "target (TTS) language: ru, en or kk")
		flagOutput       = flag.String("output", ".", "directory to save files into")
		flagOutputFile   = flag.String("output-file", "", "output filename (requires --output; ignored for multiple links)")
		flagBatchFile    = flag.String("batch-file", "", "read video links from a file, one per line (# comments and blank lines ignored)")
		flagSubs         = flag.Bool("subs", false, "download subtitles instead of audio")
		flagSubsSrt      = flag.Bool("subs-srt", false, "save subtitles as .srt (default: .vtt)")
		flagProxy        = flag.String("proxy", "", "HTTP/HTTPS proxy URL")
		flagWorker       = flag.Bool("worker", false, "route requests through the VOT worker proxy (geo bypass)")
		flagClone        = flag.Bool("clone", false, "use Yandex voice cloning (\"lively voice\"): requires --token and only works en→ru")
		flagToken        = flag.String("token", "", "Yandex account OAuth token for --clone (falls back to $VOT_TOKEN or $YANDEX_OAUTH)")
		flagOrigVolume   = flag.Float64("orig-volume", 0.3, "level of the original audio under the translation when muxing (0–1)")
		flagDuck         = flag.String("duck", "classic", "ducking mode when muxing: \"classic\" (constant) or \"smart\" (adaptive, ffmpeg sidechaincompress)")
		flagClean        = flag.Bool("clean", false, "after muxing, delete the intermediate files (downloaded audio and source), keeping only the final video (named <title>.mp4)")
		flagURLOnly      = flag.Bool("url-only", false, "print the result URL without downloading")
		flagVideoQuality = flag.String("video-quality", "best", "max source video quality for yt-dlp when muxing: best, 2160, 1440, 1080, 720 or 480")
		flagVersion      = flag.Bool("version", false, "print version and exit")
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
	if *flagBatchFile != "" {
		fileLinks, err := readLinksFile(*flagBatchFile)
		if err != nil {
			fatal(fmt.Errorf("batch file: %w", err))
		}
		links = append(links, fileLinks...)
	}
	if len(links) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	wantSubs := *flagSubs
	subsSrt := *flagSubsSrt

	// Resolve the OAuth token: --token wins, else fall back to env vars.
	token := *flagToken
	if token == "" {
		token = firstNonEmpty(os.Getenv("VOT_TOKEN"), os.Getenv("YANDEX_OAUTH"))
	}

	if !lang.IsAvailableTTS(*flagResLang) {
		fmt.Fprintf(os.Stderr, "warning: target language %q is not in the known TTS list %v; trying anyway\n", *flagResLang, lang.AvailableTTS)
	}
	if *flagOrigVolume < 0 || *flagOrigVolume > 1 {
		fatal(fmt.Errorf("--orig-volume must be between 0 and 1, got %v", *flagOrigVolume))
	}
	if *flagDuck != "classic" && *flagDuck != "smart" {
		fatal(fmt.Errorf("--duck must be \"classic\" or \"smart\", got %q", *flagDuck))
	}
	videoQuality, err := parseVideoQuality(*flagVideoQuality)
	if err != nil {
		fatal(err)
	}
	if *flagClean && !muxFlag.set {
		fmt.Fprintln(os.Stderr, "warning: --clean has no effect without --video-mux")
	}
	if *flagClone {
		if token == "" {
			fatal(errors.New("--clone requires a Yandex OAuth token: pass --token, or set $VOT_TOKEN / $YANDEX_OAUTH"))
		}
		if *flagResLang != "ru" {
			fatal(errors.New("--clone (voice cloning) only supports English→Russian; use --reslang=ru"))
		}
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
		APIToken:     token,
	})
	if err != nil {
		fatal(err)
	}

	outputFile := *flagOutputFile
	if len(links) > 1 && outputFile != "" {
		fmt.Fprintln(os.Stderr, "warning: --output-file is ignored for multiple links")
		outputFile = ""
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	app := &app{
		v:            v,
		http:         httpClient,
		lang:         *flagLang,
		resLang:      *flagResLang,
		output:       *flagOutput,
		outputFile:   outputFile,
		subsSrt:      subsSrt,
		mux:          muxFlag.set,
		video:        muxFlag.source,
		origVolume:   *flagOrigVolume,
		duckSmart:    *flagDuck == "smart",
		clone:        *flagClone,
		clean:        *flagClean,
		urlOnly:      *flagURLOnly,
		videoQuality: videoQuality,
	}

	exitCode := 0
	for i, link := range links {
		if len(links) > 1 {
			fmt.Printf("[%d/%d] ", i+1, len(links))
		}
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
		if ctx.Err() != nil { // interrupted (Ctrl-C): stop the batch
			break
		}
	}
	os.Exit(exitCode)
}

// readLinksFile reads video links from a file, one per line. Blank lines and
// lines starting with '#' are ignored.
func readLinksFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var links []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		links = append(links, line)
	}
	return links, sc.Err()
}

type app struct {
	v            *vot.Client
	http         *http.Client
	lang         string
	resLang      string
	output       string
	outputFile   string
	subsSrt      bool
	mux          bool
	video        string
	origVolume   float64
	duckSmart    bool
	clone        bool
	clean        bool
	urlOnly      bool
	videoQuality int // max source video height for yt-dlp; 0 = best
}

func (a *app) processAudio(ctx context.Context, link string) error {
	fmt.Printf("→ %s\n", link)

	reqLang := a.lang
	if a.clone {
		// Yandex voice cloning ("lively voice") only supports the en→ru pair and
		// forces the request source language to English.
		reqLang = "en"
		fmt.Println("  voice cloning (lively voice) enabled (en→ru)")
	}

	res, err := a.v.Translate(ctx, link, vot.TranslateOptions{
		RequestLang:    reqLang,
		ResponseLang:   a.resLang,
		UseLivelyVoice: a.clone,
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
	name := sanitize(resolveTitle(ctx, a.http, link, res.VideoData))
	dest := filepath.Join(a.output, fileNameOr(a.outputFile, name+".mp3"))

	n, err := downloadFile(ctx, a.http, res.URL, dest)
	if err != nil {
		return err
	}
	fmt.Printf("  saved %s (%s)\n", dest, humanSize(n))

	if a.mux {
		// Intermediates are the files we created and may delete with --clean.
		// A user-supplied source (a.video) and remote URLs are never deleted.
		intermediates := []string{dest}

		videoSource := a.video
		if videoSource == "" {
			switch {
			case res.VideoData.Host == "custom":
				videoSource = res.VideoData.URL
			case ytdlpAvailable():
				fmt.Println("  downloading source video with yt-dlp...")
				p, err := ytdlpDownload(ctx, ytdlpSourceURL(res.VideoData, link), a.output, name+".source", a.videoQuality)
				if err != nil {
					return err
				}
				videoSource = p
				intermediates = append(intermediates, p)
				fmt.Printf("  source video: %s\n", videoSource)
			default:
				return errors.New("--video-mux needs a <file|url> value for non-direct services (or install yt-dlp to fetch the source automatically)")
			}
		}

		// With --clean the muxed file is the only output, so drop the .mux infix.
		suffix := ".mux.mp4"
		if a.clean {
			suffix = ".mp4"
		}
		out := filepath.Join(a.output, name+suffix)

		duckMode := "classic"
		if a.duckSmart {
			duckMode = "smart"
		}
		fmt.Printf("  muxing into %s (%s ducking, original at %.0f%%)...\n", out, duckMode, a.origVolume*100)
		origLang := ""
		if a.lang != "" && a.lang != "auto" {
			origLang = lang.ISO6392(a.lang)
		}
		if err := muxAudio(ctx, videoSource, dest, out, muxOptions{
			OrigVolume: a.origVolume,
			Smart:      a.duckSmart,
			TransLang:  lang.ISO6392(a.resLang),
			OrigLang:   origLang,
		}); err != nil {
			return err
		}
		fmt.Printf("  muxed %s\n", out)

		if a.clean {
			for _, f := range intermediates {
				if f == out {
					continue // never delete the final output
				}
				if err := os.Remove(f); err == nil {
					fmt.Printf("  removed %s\n", f)
				}
			}
		}
	}
	return nil
}

// pickSubtitleTrack chooses which subtitle track to download: it prefers a track
// translated into resLang, falling back to the first track's translation, and
// finally to the original (untranslated) track. It returns the chosen URL and
// the language to tag the output file with.
func pickSubtitleTrack(tracks []client.SubtitleTrack, resLang string) (url, lang string) {
	if len(tracks) == 0 {
		return "", ""
	}
	track := tracks[0]
	for _, s := range tracks {
		if s.TranslatedLanguage == resLang && s.TranslatedURL != "" {
			track = s
			break
		}
	}
	if track.TranslatedURL != "" {
		return track.TranslatedURL, track.TranslatedLanguage
	}
	return track.URL, track.Language
}

func (a *app) processSubtitles(ctx context.Context, link string) error {
	fmt.Printf("→ %s\n", link)

	// Resolve video data up front for the output-file title (cheap/offline for
	// YouTube). Subtitles are polled separately so the server has time to
	// generate them.
	vd, err := a.v.GetVideoData(ctx, link)
	if err != nil {
		return err
	}
	result, err := a.v.GetSubtitlesWait(ctx, link, a.lang, func() {
		fmt.Println("  subtitles are still being generated, waiting...")
	})
	if err != nil {
		return err
	}
	if len(result.Subtitles) == 0 {
		if result.Waiting {
			return errors.New("subtitles are still being generated, try again later")
		}
		return errors.New("no subtitles available")
	}

	srcURL, srcLang := pickSubtitleTrack(result.Subtitles, a.resLang)

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
	name := sanitize(resolveTitle(ctx, a.http, link, vd))
	dest := filepath.Join(a.output, fileNameOr(a.outputFile, fmt.Sprintf("%s.%s.%s", name, srcLang, format)))
	if err := os.WriteFile(dest, converted, 0o644); err != nil {
		return err
	}
	fmt.Printf("  saved subtitles %s\n", dest)
	return nil
}

var (
	// fsUnsafeChars are characters not allowed in filenames on common OSes.
	fsUnsafeChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)
	whitespaceRun = regexp.MustCompile(`\s+`)
)

// sanitize turns an arbitrary title into a safe filename while preserving
// Unicode letters/digits (so Cyrillic and other scripts survive).
func sanitize(s string) string {
	s = strings.TrimSpace(s)
	s = fsUnsafeChars.ReplaceAllString(s, "")
	s = whitespaceRun.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_.")
	if s == "" {
		return "video"
	}
	// Cap by runes, not bytes, to avoid splitting multibyte characters.
	if r := []rune(s); len(r) > 120 {
		s = strings.Trim(string(r[:120]), "_.")
	}
	return s
}

func fileNameOr(name, def string) string {
	if name != "" {
		return name
	}
	return def
}

// parseVideoQuality maps the --video-quality flag to a yt-dlp max height. "best"
// (or empty) means no cap (0); otherwise the value must be a positive integer
// number of vertical pixels (e.g. 1080).
func parseVideoQuality(s string) (int, error) {
	if s == "" || s == "best" {
		return 0, nil
	}
	h, err := strconv.Atoi(s)
	if err != nil || h <= 0 {
		return 0, fmt.Errorf("--video-quality must be \"best\" or a positive height (e.g. 1080), got %q", s)
	}
	return h, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
