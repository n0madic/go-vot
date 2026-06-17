# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go port of the [@vot.js](https://github.com/FOSWLY/vot.js) library and the
[voice-over-translation](https://github.com/ilyhalight/voice-over-translation) userscript: it
requests Yandex voice-over translations (and subtitles) for online videos. The whole API is a
library under `pkg/`; `cmd/vot-cli/` is the user-facing utility.

The implementation is a faithful port of **@vot.js@2.4.12**. When a wire detail is unclear, the
upstream source is the ground truth and can be re-fetched without installing npm:

```sh
curl -s https://cdn.jsdelivr.net/npm/@vot.js/shared@2.4.12/dist/protos/yandex.js   # protobuf field tags
curl -s https://cdn.jsdelivr.net/npm/@vot.js/shared@2.4.12/dist/secure.js          # signing
curl -s https://cdn.jsdelivr.net/npm/@vot.js/core@2.4.12/dist/client.js            # request flow
curl -s https://cdn.jsdelivr.net/npm/@vot.js/node@2.4.12/dist/data/sites.js        # service registry
```

## Commands

```sh
go build ./...                                            # build everything
go vet ./...                                              # vet
go test ./...                                             # all tests
go test ./pkg/yaproto/                                    # one package
go test ./pkg/client/ -run TestTranslateVideoFinished     # one test
gofmt -w .                                                # format (run before finishing)

go run ./cmd/vot-cli --reslang=ru https://youtu.be/<ID>   # run the CLI
go build -o vot-cli ./cmd/vot-cli                         # build the CLI binary
```

The only external dependency is `google.golang.org/protobuf` (used solely as
`encoding/protowire`). Optional runtime tools for the CLI: `ffmpeg` (`--video-mux`) and `yt-dlp`
(auto-fetches the source video for muxing non-direct links).

## Architecture

Strict one-directional layering (no cycles). `pkg/vot` is the only package most callers need.

```
cmd/vot-cli ─▶ pkg/vot ─▶ pkg/client ─▶ pkg/{secure, yaproto, config}
                  └──────▶ pkg/service ─▶ pkg/{config}          (helpers + registry)
                  └──────▶ pkg/subs                              (subtitle conversion)
                           pkg/lang                              (language tables)
```

- **`pkg/vot`** — facade. Resolves a URL to `VideoData`, then runs the **polling loop** around the
  one-shot client. Shares a single `*http.Client` between the API client and the service fetcher.
- **`pkg/client`** — one HTTP call per method (translate / subtitles / audio / stream / cache /
  session). No retries here. `Client.worker` toggles the JSON-envelope transport; `NewWorkerClient`
  sets it. A `Doer` interface makes it testable with a fake (see `client_test.go`).
- **`pkg/yaproto`** — hand-written protobuf wire codec via `protowire`. See the invariant below.
- **`pkg/secure`** — HMAC-SHA256 signing and the session UUID.
- **`pkg/service`** — **single package** (helpers are not a subpackage, to avoid an import cycle
  with `VideoData`). A registry of ~55 services plus a `helpers` map of video-id extractors.

## How the Yandex translation algorithm works

This is the core of the port; the flow lives in `pkg/client/{session,translate}.go` and
`pkg/vot/vot.go`, with constants in `pkg/config` and signing in `pkg/secure`.

**1. Session.** Every signed request needs a session keyed by module `"video-translation"`
(`client.getSession`). A session is created via `POST /session/create` with body
`YandexSessionRequest{uuid, module}` and a single header `Vtrans-Signature: HMAC(body)`. The
response `YandexSessionResponse{secretKey, expires}` is cached until `timestamp+expires` elapses.
`uuid` is 32 random **uppercase hex** chars (NOT an RFC-4122 UUID).

**2. Signing** (`secure.SecYaHeaders`, secType is `"Vtrans"` for translate/audio/stream,
`"Vsubs"` for subtitles). Three headers are attached to each call:
- `{secType}-Signature` = `hex(HMAC-SHA256(HMACKey, body))`
- `Sec-{secType}-Sk` = `session.secretKey`
- `Sec-{secType}-Token` = `HMAC(token) + ":" + token`, where `token = uuid:path:componentVersion`

`HMACKey` and `componentVersion` are public constants baked into `pkg/config` (lifted from the
Yandex Browser client). The body that is signed is the **exact protobuf bytes** — see the invariant.

**3. Translate request** (`POST /video-translation/translate`, body
`VideoTranslationRequest`). `yaproto.NewVideoTranslationRequest` fills the constant fields the same
way upstream does: `firstRequest=true, unknown0=1, unknown2=1, unknown3=2`, duration defaults to
`config.DefaultDuration` (343) when unknown. The response is a `VideoTranslationResponse` whose
`status` drives the state machine:

| status | meaning | action |
|--------|---------|--------|
| `FINISHED` / `PART_CONTENT` | done | return the audio URL (`resp.url`, an S3 mp3 link) |
| `WAITING` / `LONG_WAITING` | in progress | not done; wait `remainingTime` and retry |
| `AUDIO_REQUESTED` | server wants the source audio | YouTube special-case (below), else wait |
| `FAILED` | untranslatable | error |
| `SESSION_REQUIRED` | needs Yandex auth | `ErrSessionRequired` |

**4. Polling** (`vot.Client.Translate`). The client does one request; the facade loops: first wait
= `min(remainingTime, 180)` seconds, subsequent waits = 30s, cancellable via `context`.

**5. Two transports.** Most links go to the Yandex protobuf API
(`api.browser.yandex.ru`). **Direct media links** — `.m3u8/.m4a/.m4v/.mpd` or the Epic CDN, detected
by `client.IsCustomLink` — instead go to the FOSWLY VOT backend (`vot.toil.cc`) as JSON
(`translateVideoVOT`). **Worker mode** (`--worker`) wraps the protobuf body in a JSON envelope
`{headers, body:[]byte}` and posts it to the worker host to bypass geo-blocking.

**6. AUDIO_REQUESTED YouTube path.** For `youtu.be/*` links the server may ask for audio; the client
calls `RequestVtransFailAudio` + uploads an empty `AudioBufferObject` (fileId =
`web_api_get_all_generating_urls_data_from_iframe`), then retries the translate with
`shouldSendFailedAudio=false`. Yandex resolves the actual audio server-side.

## Service detection & video data

`service.GetVideoData(ctx, fetcher, url)`: `GetService` matches the URL host against the ordered
`sites` registry, then the `helpers[host].id` function extracts the video id (regex per service).
- Services **without** `needExtraData` build `url = service.BaseURL + videoId` (duration is unknown,
  so 343 is sent and Yandex figures it out). ~40 services work this way.
- Services **with** `needExtraData` (Vimeo implemented; the rest registered as stubs) require an HTTP
  scrape via `helpers[host].data`; unimplemented ones return `service.ErrNotImplemented`.

To add a service, add an entry to the `helpers` map in `pkg/service/helpers.go` (and, if it needs a
scrape, a `data` func like `vimeoData`). The registry entry in `service.go` already exists for ~55
hosts.

## Critical invariant: byte-exact protobuf

The signature is computed over the serialized request body, so the Go encoder MUST produce
**byte-identical** output to upstream ts-proto, or Yandex rejects the request. This drives the
`pkg/yaproto` design:
- A field is emitted only when non-default (`!= 0 / "" / false`), matching the ts-proto conditionals.
- Field order follows the upstream `encode()` exactly — including quirks like `AudioBufferObject`
  writing field 2 (audioFile) **before** field 1 (fileId).

When changing any message, mirror the corresponding `encode`/`decode` in
`@vot.js/shared/dist/protos/yandex.js` field-for-field. `pkg/yaproto/messages_test.go` checks the
wire layout; `pkg/secure/secure_test.go` pins the HMAC vectors (cross-checkable with
`printf '%s' <input> | openssl dgst -sha256 -hmac <HMACKey>`).
