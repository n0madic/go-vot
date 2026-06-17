// Package config holds the constants required to talk to the Yandex VOT API.
//
// The values are ported verbatim from the upstream @vot.js@2.4.12 library
// (FOSWLY/vot.js, dist/data/config.js). They describe the Yandex Browser
// client that the VOT backend expects to see.
package config

const (
	// HostYandex is the primary Yandex VOT API host (protobuf endpoints).
	HostYandex = "api.browser.yandex.ru"
	// HostVOT is the FOSWLY VOT backend used for custom/direct links (JSON API).
	HostVOT = "vot.toil.cc/v1"
	// HostWorker is the worker proxy that wraps requests in JSON to bypass
	// geo restrictions.
	HostWorker = "vot-worker.toil.cc"
	// MediaProxy proxies media (audio/subtitles) downloads.
	MediaProxy = "media-proxy.toil.cc"

	// UserAgent mimics Yandex Browser on Windows.
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 YaBrowser/25.4.0.0 Safari/537.36"
	// ComponentVersion is the Yandex Browser component version embedded in the
	// per-request security token.
	ComponentVersion = "25.6.0.2259"
	// HMACKey is the public HMAC-SHA256 key used to sign request bodies and tokens.
	HMACKey = "bt8xH3VOlb4mqf0nqAibnDOoiPlXsisf"

	// DefaultDuration is sent as the video duration when it is unknown (seconds).
	DefaultDuration = 343
	// MinChunkSize is the minimum audio chunk size for partial audio uploads.
	MinChunkSize = 5295308
	// LibVersion is the upstream @vot.js version this port targets.
	LibVersion = "2.4.12"
)

// SessionModuleVideoTranslation is the session module name used for translation,
// subtitles and stream requests.
const SessionModuleVideoTranslation = "video-translation"
