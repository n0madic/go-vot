package yaproto

// VideoTranslationStatus mirrors the status codes returned by the Yandex VOT
// translate endpoint (ported from @vot.js/core/types/yandex).
type VideoTranslationStatus int32

const (
	StatusFailed          VideoTranslationStatus = 0
	StatusFinished        VideoTranslationStatus = 1
	StatusWaiting         VideoTranslationStatus = 2
	StatusLongWaiting     VideoTranslationStatus = 3
	StatusPartContent     VideoTranslationStatus = 5
	StatusAudioRequested  VideoTranslationStatus = 6
	StatusSessionRequired VideoTranslationStatus = 7
)

// String returns a human-readable name for the status.
func (s VideoTranslationStatus) String() string {
	switch s {
	case StatusFailed:
		return "FAILED"
	case StatusFinished:
		return "FINISHED"
	case StatusWaiting:
		return "WAITING"
	case StatusLongWaiting:
		return "LONG_WAITING"
	case StatusPartContent:
		return "PART_CONTENT"
	case StatusAudioRequested:
		return "AUDIO_REQUESTED"
	case StatusSessionRequired:
		return "SESSION_REQUIRED"
	default:
		return "UNKNOWN"
	}
}

// StreamInterval mirrors the stream translation interval codes.
type StreamInterval int32

const (
	StreamNoConnection StreamInterval = 0
	StreamTranslating  StreamInterval = 10
	StreamStreaming    StreamInterval = 20
)

// AudioDownloadType enumerates the fileId values used when uploading audio for
// services that require it (e.g. YouTube). Ported from @vot.js/core/types/yandex.
type AudioDownloadType string

const (
	AudioWebAPIVideoSrcFromIframe              AudioDownloadType = "web_api_video_src_from_iframe"
	AudioWebAPIVideoSrc                        AudioDownloadType = "web_api_video_src"
	AudioWebAPIGetAllGeneratingURLsFromIframe  AudioDownloadType = "web_api_get_all_generating_urls_data_from_iframe"
	AudioWebAPIGetAllGeneratingURLsFromIframeT AudioDownloadType = "web_api_get_all_generating_urls_data_from_iframe_tmp_exp"
	AudioWebAPIReplacedFetchInsideIframe       AudioDownloadType = "web_api_replaced_fetch_inside_iframe"
	AudioAndroidAPI                            AudioDownloadType = "android_api"
	AudioWebAPISlow                            AudioDownloadType = "web_api_slow"
	AudioWebAPIStealSigAndN                    AudioDownloadType = "web_api_steal_sig_and_n"
	AudioWebAPICombined                        AudioDownloadType = "web_api_get_all_generating_urls_data_from_iframe,web_api_steal_sig_and_n"
)
