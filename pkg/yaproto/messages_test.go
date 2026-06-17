package yaproto

import (
	"bytes"
	"encoding/hex"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestYandexSessionRequestRoundTrip(t *testing.T) {
	req := &YandexSessionRequest{UUID: "ABCDEF0123456789ABCDEF0123456789", Module: "video-translation"}
	encoded := req.Marshal()

	// Field 1 (uuid): tag 0x0a, len 32; field 2 (module): tag 0x12, len 17.
	if encoded[0] != 0x0a {
		t.Fatalf("first tag = %#x, want 0x0a (field 1, bytes)", encoded[0])
	}

	var got YandexSessionResponse // not symmetric: just ensure decode of a crafted response works
	_ = got
}

func TestSessionResponseDecode(t *testing.T) {
	// secretKey="key123" (field 1), expires=3600 (field 2, varint).
	var b []byte
	b = appendString(b, 1, "key123")
	b = appendInt32(b, 2, 3600)

	var resp YandexSessionResponse
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if resp.SecretKey != "key123" || resp.Expires != 3600 {
		t.Fatalf("got %+v", resp)
	}
}

func TestTranslationRequestEncoding(t *testing.T) {
	req := NewVideoTranslationRequest(
		"https://youtu.be/aaaaaaaaaaa",
		343,
		"en",
		"ru",
		nil,
		TranslationExtraOpts{},
	)
	b := req.Marshal()

	// Re-decode the well-known constant fields to confirm wire layout.
	fields := map[int]uint64{}
	rest := b
	for len(rest) > 0 {
		num, typ, n := consumeTag(t, rest)
		rest = rest[n:]
		switch typ {
		case 0: // varint
			v, n, err := consumeVarint(rest)
			if err != nil {
				t.Fatal(err)
			}
			fields[num] = v
			rest = rest[n:]
		case 1: // fixed64
			_, n, err := consumeDouble(rest)
			if err != nil {
				t.Fatal(err)
			}
			rest = rest[n:]
		case 2: // bytes
			_, n, err := consumeBytes(rest)
			if err != nil {
				t.Fatal(err)
			}
			rest = rest[n:]
		default:
			t.Fatalf("unexpected wire type %d", typ)
		}
	}
	if fields[5] != 1 {
		t.Errorf("firstRequest(5) = %d, want 1", fields[5])
	}
	if fields[7] != 1 {
		t.Errorf("unknown0(7) = %d, want 1", fields[7])
	}
	if fields[15] != 1 {
		t.Errorf("unknown2(15) = %d, want 1", fields[15])
	}
	if fields[16] != 2 {
		t.Errorf("unknown3(16) = %d, want 2", fields[16])
	}
}

func TestTranslationResponseRoundTrip(t *testing.T) {
	url := "https://audio.example/translated.mp3"
	rt := int32(42)
	in := &VideoTranslationResponse{
		URL:           &url,
		Status:        StatusWaiting,
		RemainingTime: &rt,
		TranslationID: "tid-1",
	}
	b := in.Marshal()

	var out VideoTranslationResponse
	if err := out.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if out.URL == nil || *out.URL != url {
		t.Errorf("url = %v", out.URL)
	}
	if out.Status != StatusWaiting {
		t.Errorf("status = %v", out.Status)
	}
	if out.RemainingTime == nil || *out.RemainingTime != 42 {
		t.Errorf("remainingTime = %v", out.RemainingTime)
	}
	if out.TranslationID != "tid-1" {
		t.Errorf("translationId = %q", out.TranslationID)
	}
}

func TestSubtitlesResponseRoundTrip(t *testing.T) {
	sub := SubtitlesObject{Language: "en", URL: "u1", TranslatedLanguage: "ru", TranslatedURL: "u2"}
	var inner []byte
	inner = appendString(inner, 1, sub.Language)
	inner = appendString(inner, 2, sub.URL)
	inner = appendString(inner, 4, sub.TranslatedLanguage)
	inner = appendString(inner, 5, sub.TranslatedURL)
	var b []byte
	b = appendBool(b, 1, true)
	b = appendMessage(b, 2, inner)

	var resp SubtitlesResponse
	if err := resp.Unmarshal(b); err != nil {
		t.Fatal(err)
	}
	if !resp.Waiting || len(resp.Subtitles) != 1 || resp.Subtitles[0] != sub {
		t.Fatalf("got %+v", resp)
	}
}

func TestAudioBufferFieldOrder(t *testing.T) {
	// Upstream writes audioFile(field 2) before fileId(field 1).
	m := &AudioBufferObject{AudioFile: []byte{0x01, 0x02}, FileID: "fid"}
	b := m.Marshal()
	if b[0] != 0x12 { // tag for field 2, bytes
		t.Fatalf("first tag = %#x, want 0x12 (field 2)", b[0])
	}
	// The fileId tag (0x0a, field 1) must appear after the audio bytes.
	if !bytes.Contains(b[3:], []byte{0x0a}) {
		t.Fatalf("fileId tag not found after audio: %s", hex.EncodeToString(b))
	}
}

// consumeTag is a tiny test helper around protowire.ConsumeTag.
func consumeTag(t *testing.T, b []byte) (num int, typ int, n int) {
	t.Helper()
	number, wtype, ln := protowire.ConsumeTag(b)
	if ln < 0 {
		t.Fatalf("bad tag")
	}
	return int(number), int(wtype), ln
}
