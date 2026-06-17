// Package yaproto implements byte-exact protobuf wire encoding/decoding for the
// Yandex VOT messages.
//
// The encoders intentionally mirror the upstream ts-proto generated code in
// @vot.js/shared/dist/protos/yandex.js field-by-field (including the conditional
// "encode only if non-default" logic and the exact field ordering), because the
// request body is HMAC-signed: a byte-for-byte identical encoding is required
// for the signature — and therefore the request — to be accepted by Yandex.
package yaproto

import (
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

// --- low-level append helpers -------------------------------------------------

func appendString(b []byte, num protowire.Number, v string) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendString(b, v)
}

func appendBytes(b []byte, num protowire.Number, v []byte) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, v)
}

func appendBool(b []byte, num protowire.Number, v bool) []byte {
	b = protowire.AppendTag(b, num, protowire.VarintType)
	return protowire.AppendVarint(b, protowire.EncodeBool(v))
}

// appendInt32 encodes an int32 field as a varint using protobuf int32 semantics
// (negative values are sign-extended to 64 bits).
func appendInt32(b []byte, num protowire.Number, v int32) []byte {
	b = protowire.AppendTag(b, num, protowire.VarintType)
	return protowire.AppendVarint(b, uint64(int64(v)))
}

func appendDouble(b []byte, num protowire.Number, v float64) []byte {
	b = protowire.AppendTag(b, num, protowire.Fixed64Type)
	return protowire.AppendFixed64(b, math.Float64bits(v))
}

func appendMessage(b []byte, num protowire.Number, sub []byte) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, sub)
}

// --- low-level consume helpers ------------------------------------------------

// fieldIter walks the fields of a protobuf message. It calls fn for each field;
// fn should consume the field value for known fields. Unknown fields and fields
// fn does not handle are skipped automatically.
func fieldIter(b []byte, fn func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error)) error {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]
		consumed, handled, err := fn(num, typ, b)
		if err != nil {
			return err
		}
		if !handled {
			consumed = protowire.ConsumeFieldValue(num, typ, b)
			if consumed < 0 {
				return protowire.ParseError(consumed)
			}
		}
		b = b[consumed:]
	}
	return nil
}

func consumeString(b []byte) (string, int, error) {
	v, n := protowire.ConsumeString(b)
	if n < 0 {
		return "", 0, protowire.ParseError(n)
	}
	return v, n, nil
}

func consumeBytes(b []byte) ([]byte, int, error) {
	v, n := protowire.ConsumeBytes(b)
	if n < 0 {
		return nil, 0, protowire.ParseError(n)
	}
	return append([]byte(nil), v...), n, nil
}

func consumeVarint(b []byte) (uint64, int, error) {
	v, n := protowire.ConsumeVarint(b)
	if n < 0 {
		return 0, 0, protowire.ParseError(n)
	}
	return v, n, nil
}

func consumeDouble(b []byte) (float64, int, error) {
	v, n := protowire.ConsumeFixed64(b)
	if n < 0 {
		return 0, 0, protowire.ParseError(n)
	}
	return math.Float64frombits(v), n, nil
}

func ptr[T any](v T) *T { return &v }

// --- VideoTranslationHelpObject ----------------------------------------------

// TranslationHelp is a hint passed to Yandex to improve translation, pointing at
// a raw video or subtitles file.
type TranslationHelp struct {
	Target    string // "video_file_url" or "subtitles_file_url"
	TargetURL string
}

func (m *TranslationHelp) Marshal() []byte {
	var b []byte
	if m.Target != "" {
		b = appendString(b, 1, m.Target)
	}
	if m.TargetURL != "" {
		b = appendString(b, 2, m.TargetURL)
	}
	return b
}

func (m *TranslationHelp) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeString(b)
			m.Target = v
			return n, true, err
		case 2:
			v, n, err := consumeString(b)
			m.TargetURL = v
			return n, true, err
		}
		return 0, false, nil
	})
}

// --- VideoTranslationRequest --------------------------------------------------

// TranslationExtraOpts mirrors the optional flags accepted by the upstream
// encodeTranslationRequest.
type TranslationExtraOpts struct {
	ForceSourceLang bool
	WasStream       bool
	VideoTitle      string
	BypassCache     bool
	UseLivelyVoice  bool
}

// VideoTranslationRequest is the body of POST /video-translation/translate.
type VideoTranslationRequest struct {
	URL              string
	DeviceID         *string
	FirstRequest     bool
	Duration         float64
	Unknown0         int32
	Language         string
	ForceSourceLang  bool
	Unknown1         int32
	TranslationHelp  []TranslationHelp
	WasStream        bool
	ResponseLanguage string
	Unknown2         int32
	Unknown3         int32
	BypassCache      bool
	UseLivelyVoice   bool
	VideoTitle       string
}

// NewVideoTranslationRequest builds a request with the constant fields set the
// same way the upstream YandexVOTProtobuf.encodeTranslationRequest does
// (unknown0=1, unknown2=1, unknown3=2, firstRequest defaults to true).
func NewVideoTranslationRequest(url string, duration float64, reqLang, resLang string, help []TranslationHelp, opts TranslationExtraOpts) *VideoTranslationRequest {
	return &VideoTranslationRequest{
		URL:              url,
		FirstRequest:     true, // always true in the upstream flows
		Duration:         duration,
		Unknown0:         1,
		Language:         reqLang,
		ForceSourceLang:  opts.ForceSourceLang,
		Unknown1:         0,
		TranslationHelp:  help,
		WasStream:        opts.WasStream,
		ResponseLanguage: resLang,
		Unknown2:         1,
		Unknown3:         2,
		BypassCache:      opts.BypassCache,
		UseLivelyVoice:   opts.UseLivelyVoice,
		VideoTitle:       opts.VideoTitle,
	}
}

func (m *VideoTranslationRequest) Marshal() []byte {
	var b []byte
	if m.URL != "" {
		b = appendString(b, 3, m.URL)
	}
	if m.DeviceID != nil {
		b = appendString(b, 4, *m.DeviceID)
	}
	if m.FirstRequest {
		b = appendBool(b, 5, m.FirstRequest)
	}
	if m.Duration != 0 {
		b = appendDouble(b, 6, m.Duration)
	}
	if m.Unknown0 != 0 {
		b = appendInt32(b, 7, m.Unknown0)
	}
	if m.Language != "" {
		b = appendString(b, 8, m.Language)
	}
	if m.ForceSourceLang {
		b = appendBool(b, 9, m.ForceSourceLang)
	}
	if m.Unknown1 != 0 {
		b = appendInt32(b, 10, m.Unknown1)
	}
	for i := range m.TranslationHelp {
		b = appendMessage(b, 11, m.TranslationHelp[i].Marshal())
	}
	if m.WasStream {
		b = appendBool(b, 13, m.WasStream)
	}
	if m.ResponseLanguage != "" {
		b = appendString(b, 14, m.ResponseLanguage)
	}
	if m.Unknown2 != 0 {
		b = appendInt32(b, 15, m.Unknown2)
	}
	if m.Unknown3 != 0 {
		b = appendInt32(b, 16, m.Unknown3)
	}
	if m.BypassCache {
		b = appendBool(b, 17, m.BypassCache)
	}
	if m.UseLivelyVoice {
		b = appendBool(b, 18, m.UseLivelyVoice)
	}
	if m.VideoTitle != "" {
		b = appendString(b, 19, m.VideoTitle)
	}
	return b
}

// --- VideoTranslationResponse -------------------------------------------------

// VideoTranslationResponse is the decoded body returned by the translate endpoint.
type VideoTranslationResponse struct {
	URL           *string
	Duration      *float64
	Status        VideoTranslationStatus
	RemainingTime *int32
	TranslationID string
	Language      *string
	Message       *string
	IsLivelyVoice bool
	ShouldRetry   *int32
}

func (m *VideoTranslationResponse) Marshal() []byte {
	var b []byte
	if m.URL != nil {
		b = appendString(b, 1, *m.URL)
	}
	if m.Duration != nil {
		b = appendDouble(b, 2, *m.Duration)
	}
	if m.Status != 0 {
		b = appendInt32(b, 4, int32(m.Status))
	}
	if m.RemainingTime != nil {
		b = appendInt32(b, 5, *m.RemainingTime)
	}
	if m.TranslationID != "" {
		b = appendString(b, 7, m.TranslationID)
	}
	if m.Language != nil {
		b = appendString(b, 8, *m.Language)
	}
	if m.Message != nil {
		b = appendString(b, 9, *m.Message)
	}
	if m.IsLivelyVoice {
		b = appendBool(b, 10, m.IsLivelyVoice)
	}
	if m.ShouldRetry != nil {
		b = appendInt32(b, 12, *m.ShouldRetry)
	}
	return b
}

func (m *VideoTranslationResponse) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeString(b)
			m.URL = ptr(v)
			return n, true, err
		case 2:
			v, n, err := consumeDouble(b)
			m.Duration = ptr(v)
			return n, true, err
		case 4:
			v, n, err := consumeVarint(b)
			m.Status = VideoTranslationStatus(int32(v))
			return n, true, err
		case 5:
			v, n, err := consumeVarint(b)
			m.RemainingTime = ptr(int32(v))
			return n, true, err
		case 7:
			v, n, err := consumeString(b)
			m.TranslationID = v
			return n, true, err
		case 8:
			v, n, err := consumeString(b)
			m.Language = ptr(v)
			return n, true, err
		case 9:
			v, n, err := consumeString(b)
			m.Message = ptr(v)
			return n, true, err
		case 10:
			v, n, err := consumeVarint(b)
			m.IsLivelyVoice = v != 0
			return n, true, err
		case 12:
			v, n, err := consumeVarint(b)
			m.ShouldRetry = ptr(int32(v))
			return n, true, err
		}
		return 0, false, nil
	})
}

// --- VideoTranslationCache ----------------------------------------------------

type VideoTranslationCacheItem struct {
	Status        int32
	RemainingTime *int32
	Message       *string
}

func (m *VideoTranslationCacheItem) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeVarint(b)
			m.Status = int32(v)
			return n, true, err
		case 2:
			v, n, err := consumeVarint(b)
			m.RemainingTime = ptr(int32(v))
			return n, true, err
		case 3:
			v, n, err := consumeString(b)
			m.Message = ptr(v)
			return n, true, err
		}
		return 0, false, nil
	})
}

type VideoTranslationCacheRequest struct {
	URL              string
	Duration         float64
	Language         string
	ResponseLanguage string
}

func (m *VideoTranslationCacheRequest) Marshal() []byte {
	var b []byte
	if m.URL != "" {
		b = appendString(b, 1, m.URL)
	}
	if m.Duration != 0 {
		b = appendDouble(b, 2, m.Duration)
	}
	if m.Language != "" {
		b = appendString(b, 3, m.Language)
	}
	if m.ResponseLanguage != "" {
		b = appendString(b, 4, m.ResponseLanguage)
	}
	return b
}

type VideoTranslationCacheResponse struct {
	Default *VideoTranslationCacheItem
	Cloning *VideoTranslationCacheItem
}

func (m *VideoTranslationCacheResponse) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeBytes(b)
			if err != nil {
				return n, true, err
			}
			m.Default = &VideoTranslationCacheItem{}
			return n, true, m.Default.Unmarshal(v)
		case 2:
			v, n, err := consumeBytes(b)
			if err != nil {
				return n, true, err
			}
			m.Cloning = &VideoTranslationCacheItem{}
			return n, true, m.Cloning.Unmarshal(v)
		}
		return 0, false, nil
	})
}

// --- audio --------------------------------------------------------------------

// AudioBufferObject holds a complete audio file plus its identifier.
type AudioBufferObject struct {
	AudioFile []byte
	FileID    string
}

// Marshal mirrors the upstream field ordering (audioFile=2 is written before
// fileId=1).
func (m *AudioBufferObject) Marshal() []byte {
	var b []byte
	if len(m.AudioFile) != 0 {
		b = appendBytes(b, 2, m.AudioFile)
	}
	if m.FileID != "" {
		b = appendString(b, 1, m.FileID)
	}
	return b
}

// PartialAudioBufferObject holds one chunk of an audio file.
type PartialAudioBufferObject struct {
	AudioFile []byte
	ChunkID   int32
}

func (m *PartialAudioBufferObject) Marshal() []byte {
	var b []byte
	if len(m.AudioFile) != 0 {
		b = appendBytes(b, 2, m.AudioFile)
	}
	if m.ChunkID != 0 {
		b = appendInt32(b, 1, m.ChunkID)
	}
	return b
}

// ChunkAudioObject wraps a partial audio buffer with chunk metadata.
type ChunkAudioObject struct {
	AudioBuffer      *PartialAudioBufferObject
	AudioPartsLength int32
	FileID           string
	Version          int32
}

func (m *ChunkAudioObject) Marshal() []byte {
	var b []byte
	if m.AudioBuffer != nil {
		b = appendMessage(b, 1, m.AudioBuffer.Marshal())
	}
	if m.AudioPartsLength != 0 {
		b = appendInt32(b, 2, m.AudioPartsLength)
	}
	if m.FileID != "" {
		b = appendString(b, 3, m.FileID)
	}
	if m.Version != 0 {
		b = appendInt32(b, 4, m.Version)
	}
	return b
}

// VideoTranslationAudioRequest is the body of PUT /video-translation/audio.
type VideoTranslationAudioRequest struct {
	TranslationID    string
	URL              string
	PartialAudioInfo *ChunkAudioObject
	AudioInfo        *AudioBufferObject
}

func (m *VideoTranslationAudioRequest) Marshal() []byte {
	var b []byte
	if m.TranslationID != "" {
		b = appendString(b, 1, m.TranslationID)
	}
	if m.URL != "" {
		b = appendString(b, 2, m.URL)
	}
	if m.PartialAudioInfo != nil {
		b = appendMessage(b, 4, m.PartialAudioInfo.Marshal())
	}
	if m.AudioInfo != nil {
		b = appendMessage(b, 6, m.AudioInfo.Marshal())
	}
	return b
}

// VideoTranslationAudioResponse is the decoded audio-upload response.
type VideoTranslationAudioResponse struct {
	Status          int32
	RemainingChunks []string
}

func (m *VideoTranslationAudioResponse) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeVarint(b)
			m.Status = int32(v)
			return n, true, err
		case 2:
			v, n, err := consumeString(b)
			if err == nil {
				m.RemainingChunks = append(m.RemainingChunks, v)
			}
			return n, true, err
		}
		return 0, false, nil
	})
}

// --- subtitles ----------------------------------------------------------------

// SubtitlesObject describes one available subtitle track.
type SubtitlesObject struct {
	Language           string
	URL                string
	TranslatedLanguage string
	TranslatedURL      string
}

func (m *SubtitlesObject) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeString(b)
			m.Language = v
			return n, true, err
		case 2:
			v, n, err := consumeString(b)
			m.URL = v
			return n, true, err
		case 4:
			v, n, err := consumeString(b)
			m.TranslatedLanguage = v
			return n, true, err
		case 5:
			v, n, err := consumeString(b)
			m.TranslatedURL = v
			return n, true, err
		}
		return 0, false, nil
	})
}

// SubtitlesRequest is the body of POST /video-subtitles/get-subtitles.
type SubtitlesRequest struct {
	URL      string
	Language string
}

func (m *SubtitlesRequest) Marshal() []byte {
	var b []byte
	if m.URL != "" {
		b = appendString(b, 1, m.URL)
	}
	if m.Language != "" {
		b = appendString(b, 2, m.Language)
	}
	return b
}

// SubtitlesResponse is the decoded subtitles response.
type SubtitlesResponse struct {
	Waiting   bool
	Subtitles []SubtitlesObject
}

func (m *SubtitlesResponse) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeVarint(b)
			m.Waiting = v != 0
			return n, true, err
		case 2:
			v, n, err := consumeBytes(b)
			if err != nil {
				return n, true, err
			}
			var sub SubtitlesObject
			if err := sub.Unmarshal(v); err != nil {
				return n, true, err
			}
			m.Subtitles = append(m.Subtitles, sub)
			return n, true, nil
		}
		return 0, false, nil
	})
}

// --- stream -------------------------------------------------------------------

// StreamTranslationObject holds a translated stream fragment.
type StreamTranslationObject struct {
	URL       string
	Timestamp string
}

func (m *StreamTranslationObject) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeString(b)
			m.URL = v
			return n, true, err
		case 2:
			v, n, err := consumeString(b)
			m.Timestamp = v
			return n, true, err
		}
		return 0, false, nil
	})
}

// StreamTranslationRequest is the body of POST /stream-translation/translate-stream.
type StreamTranslationRequest struct {
	URL              string
	Language         string
	ResponseLanguage string
	Unknown0         int32
	Unknown1         int32
}

func (m *StreamTranslationRequest) Marshal() []byte {
	var b []byte
	if m.URL != "" {
		b = appendString(b, 1, m.URL)
	}
	if m.Language != "" {
		b = appendString(b, 2, m.Language)
	}
	if m.ResponseLanguage != "" {
		b = appendString(b, 3, m.ResponseLanguage)
	}
	if m.Unknown0 != 0 {
		b = appendInt32(b, 5, m.Unknown0)
	}
	if m.Unknown1 != 0 {
		b = appendInt32(b, 6, m.Unknown1)
	}
	return b
}

// StreamTranslationResponse is the decoded stream response.
type StreamTranslationResponse struct {
	Interval       StreamInterval
	TranslatedInfo *StreamTranslationObject
	PingID         *int32
}

func (m *StreamTranslationResponse) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeVarint(b)
			m.Interval = StreamInterval(int32(v))
			return n, true, err
		case 2:
			v, n, err := consumeBytes(b)
			if err != nil {
				return n, true, err
			}
			m.TranslatedInfo = &StreamTranslationObject{}
			return n, true, m.TranslatedInfo.Unmarshal(v)
		case 3:
			v, n, err := consumeVarint(b)
			m.PingID = ptr(int32(v))
			return n, true, err
		}
		return 0, false, nil
	})
}

// StreamPingRequest is the body of POST /stream-translation/ping-stream.
type StreamPingRequest struct {
	PingID int32
}

func (m *StreamPingRequest) Marshal() []byte {
	var b []byte
	if m.PingID != 0 {
		b = appendInt32(b, 1, m.PingID)
	}
	return b
}

// --- session ------------------------------------------------------------------

// YandexSessionRequest is the body of POST /session/create.
type YandexSessionRequest struct {
	UUID   string
	Module string
}

func (m *YandexSessionRequest) Marshal() []byte {
	var b []byte
	if m.UUID != "" {
		b = appendString(b, 1, m.UUID)
	}
	if m.Module != "" {
		b = appendString(b, 2, m.Module)
	}
	return b
}

// YandexSessionResponse is the decoded session response.
type YandexSessionResponse struct {
	SecretKey string
	Expires   int32
}

func (m *YandexSessionResponse) Unmarshal(b []byte) error {
	return fieldIter(b, func(num protowire.Number, typ protowire.Type, b []byte) (int, bool, error) {
		switch num {
		case 1:
			v, n, err := consumeString(b)
			m.SecretKey = v
			return n, true, err
		case 2:
			v, n, err := consumeVarint(b)
			m.Expires = int32(v)
			return n, true, err
		}
		return 0, false, nil
	})
}
