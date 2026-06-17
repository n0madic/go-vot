# go-vot

A Go port of [voice-over-translation](https://github.com/ilyhalight/voice-over-translation)
and the [@vot.js](https://github.com/FOSWLY/vot.js) library: it requests Yandex
voice-over translations (and subtitles) for online videos.

The whole API is available as a library under `pkg/`, plus a `vot-cli` utility
under `cmd/vot-cli/`.

> Created for research/educational purposes, mirroring the upstream project's
> stance. Not affiliated with Yandex.

## Features

- Full Yandex VOT protocol: session creation, HMAC-SHA256 request signing,
  protobuf wire format (byte-exact with the upstream encoder), and all endpoints
  (translate, audio upload, subtitles, cache, stream, ping).
- Two transports: the Yandex protobuf API and the FOSWLY VOT backend (for direct
  media links), plus a worker-proxy mode for geo-blocked regions.
- Service detection for ~55 video services (YouTube and all simple URL-pattern
  services work out of the box; Vimeo resolves duration via its API; the rest are
  registered with an extensible helper interface).
- Subtitle conversion between Yandex JSON, SRT and VTT.
- CLI: polling with progress, audio (mp3) and subtitle download (files named by
  the video title, resolved via yt-dlp or YouTube oEmbed), optional ffmpeg
  muxing that mixes the translation over the original audio (with a second,
  untouched original track) — the real Yandex voice-over experience. Ducking is
  either `classic` (constant) or `smart` (adaptive, via `sidechaincompress`).

## Install

```sh
go install github.com/n0madic/go-vot/cmd/vot-cli@latest
```

Or build locally:

```sh
git clone https://github.com/n0madic/go-vot && cd go-vot
go build -o vot-cli ./cmd/vot-cli
```

`--video-mux` requires `ffmpeg` in `PATH`.

## CLI usage

```
vot-cli [options] <link> [link2 ...]

  --lang=<src>          source video language (default: auto)
  --reslang=<dst>       target TTS language: ru, en or kk (default: ru)
  --output=<dir>        directory to save files into (default: .)
  --output-file=<name>  output filename (requires --output; ignored for many links)
  --batch-file=<path>   read links from a file, one per line (# comments ignored)
  --subs                download subtitles instead of audio
  --subs-srt            save subtitles as .srt (default: .vtt)
  --proxy=<url>         HTTP/HTTPS proxy URL
  --worker              route requests through the VOT worker proxy (geo bypass)
  --clone               use Yandex voice cloning ("lively voice"); needs a token,
                        only works English→Russian
  --token=<oauth>       Yandex account OAuth token for --clone
                        (falls back to $VOT_TOKEN or $YANDEX_OAUTH)
  --video-mux[=<file|url>] mux the translation over the source video via ffmpeg;
                        bare --video-mux fetches the source with yt-dlp (or the
                        media URL for direct links); --video-mux=<file|url> sets
                        the source explicitly
  --orig-volume=<0..1>  level of the original audio under the translation
                        when muxing (default: 0.3)
  --clean               after muxing, delete the intermediate files (downloaded
                        audio + source), keeping only the final <title>.mp4
  --duck=<classic|smart> ducking mode when muxing (default: classic):
                        classic — original at a constant --orig-volume;
                        smart — original kept at the --orig-volume baseline and
                        dynamically ducked (ffmpeg sidechaincompress) only while
                        the translation is speaking
  --video-quality=<q>   max source video quality for yt-dlp when muxing:
                        best (default), 2160, 1440, 1080, 720 or 480 — caps the
                        downloaded resolution (smaller files, and a way around
                        flaky 4K formats); falls back to the best stream if the
                        cap can't be met
  --url-only            print the result URL without downloading
  --version             print version
```

### Examples

```sh
# Translate a YouTube video to Russian and download the mp3
vot-cli --reslang=ru https://youtu.be/dQw4w9WgXcQ

# Batch: several links at once, or from a file
vot-cli --reslang=ru https://youtu.be/ID1 https://youtu.be/ID2
vot-cli --reslang=ru --batch-file=links.txt

# Download translated subtitles as SRT
vot-cli --subs --subs-srt --reslang=ru https://youtu.be/dQw4w9WgXcQ

# Behind a proxy / from a geo-blocked region
vot-cli --worker --reslang=en https://youtu.be/dQw4w9WgXcQ

# Mux translated audio into a direct .mp4 link
vot-cli --video-mux --reslang=ru "https://example.com/clip.mp4"

# Mux into a YouTube video, capping the downloaded source at 1080p
vot-cli --video-mux --video-quality=1080 --reslang=ru https://youtu.be/dQw4w9WgXcQ

# Voice cloning ("lively voice") — English→Russian, needs a Yandex OAuth token
export YANDEX_OAUTH="y0_..."   # or VOT_TOKEN; --token overrides both
vot-cli --clone --reslang=ru https://youtu.be/dQw4w9WgXcQ
```

> Yandex geo-restricts some regions (e.g. UA/LV/LT). Use `--proxy` or `--worker`
> if direct requests fail.

> **Voice cloning** (`--clone`) is Yandex's "lively voice" that preserves the
> original speaker's voice. It only supports the **English→Russian** pair and
> requires a **Yandex account OAuth token** — pass `--token`, or set `$VOT_TOKEN`
> / `$YANDEX_OAUTH`. The source language is forced to English automatically.
> Without a valid token Yandex rejects the cloned-voice request.

## Library usage

```go
package main

import (
	"context"
	"fmt"

	"github.com/n0madic/go-vot/pkg/vot"
)

func main() {
	v, err := vot.New(vot.Options{ResponseLang: "ru"})
	if err != nil {
		panic(err)
	}

	res, err := v.Translate(context.Background(), "https://youtu.be/dQw4w9WgXcQ", vot.TranslateOptions{
		OnProgress: func(status string, remaining int32) {
			fmt.Printf("%s, waiting ~%ds\n", status, remaining)
		},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("translated audio:", res.URL)
}
```

### Packages

| Package | Purpose |
|---------|---------|
| `pkg/vot` | High-level facade: resolve URL, translate with polling, get subtitles |
| `pkg/client` | Low-level Yandex VOT API client (sessions, all endpoints, worker mode) |
| `pkg/service` | Service detection registry and video-data extraction |
| `pkg/yaproto` | Byte-exact protobuf wire encoding/decoding |
| `pkg/secure` | HMAC-SHA256 request signing and session UUID generation |
| `pkg/subs` | Subtitle conversion (JSON ↔ SRT ↔ VTT) |
| `pkg/lang` | Supported languages and ISO 639 mapping |
| `pkg/config` | API hosts, keys and version constants |

## Extending service support

Some services that need site-specific HTTP scraping (e.g. Yandex.Disk, Reddit,
Patreon) are registered in `pkg/service` but return `ErrNotImplemented` until a
helper is added. To support one, add an entry to the `helpers` map in
`pkg/service/helpers.go` implementing the `id` (and optionally `data`) function;
the `id` extractor receives a context and HTTP `Fetcher` so it can resolve ids
that require a network round-trip (as Twitch clips and mail.ru embeds do).

## Testing

```sh
go test ./...
```

Unit tests cover the protobuf round-trip and wire layout, the signing vectors,
service detection / video-id extraction, the client status machine, and subtitle
conversion.

## Credits

- [voice-over-translation](https://github.com/ilyhalight/voice-over-translation) — original userscript
- [@vot.js](https://github.com/FOSWLY/vot.js) — the reference TypeScript library this port follows
- [vot-cli](https://github.com/FOSWLY/vot-cli) — the reference CLI

Licensed under MIT.
