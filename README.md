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
- CLI: polling with progress, audio (mp3) and subtitle download, optional ffmpeg
  muxing into the source video.

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
  --output-file=<name>  output filename (requires --output)
  --subs                download subtitles instead of audio
  --subs-srt            save subtitles as .srt (default: .vtt)
  --proxy=<url>         HTTP/HTTPS proxy URL
  --worker              route requests through the VOT worker proxy (geo bypass)
  --video-mux[=<file|url>] mux translated audio into the source video via ffmpeg;
                        bare --video-mux fetches the source with yt-dlp (or the
                        media URL for direct links); --video-mux=<file|url> sets
                        the source explicitly
  --url-only            print the result URL without downloading
  --version             print version
```

### Examples

```sh
# Translate a YouTube video to Russian and download the mp3
vot-cli --reslang=ru https://youtu.be/dQw4w9WgXcQ

# Download translated subtitles as SRT
vot-cli --subs --subs-srt --reslang=ru https://youtu.be/dQw4w9WgXcQ

# Behind a proxy / from a geo-blocked region
vot-cli --worker --reslang=en https://youtu.be/dQw4w9WgXcQ

# Mux translated audio into a direct .mp4 link
vot-cli --video-mux --reslang=ru "https://example.com/clip.mp4"
```

> Yandex geo-restricts some regions (e.g. UA/LV/LT). Use `--proxy` or `--worker`
> if direct requests fail.

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

Services that need site-specific HTTP scraping (e.g. Kick, Yandex.Disk) are
registered in `pkg/service` but return `ErrNotImplemented` until a helper is
added. To support one, add an entry to the `helpers` map in
`pkg/service/helpers.go` implementing the `id` (and optionally `data`) function.

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
