package secure

import (
	"strings"
	"testing"

	"github.com/n0madic/go-vot/pkg/config"
)

func TestSignatureVectors(t *testing.T) {
	// Reference values computed with:
	//   printf '%s' <input> | openssl dgst -sha256 -hmac <HMACKey> -hex
	cases := map[string]string{
		"test":              "ef02ef65d4b0c97334b2e2840177ba354f5b7e530200636dfe5b969eda9442e4",
		"video-translation": "6386fcffe4724170489aa84fbf5a54eb2bbdb0b1bc0cad2dfed3d81af0dd4cba",
	}
	for input, want := range cases {
		if got := Signature([]byte(input)); got != want {
			t.Errorf("Signature(%q) = %s, want %s", input, got, want)
		}
	}
}

func TestSecYaHeaders(t *testing.T) {
	session := Session{SecretKey: "sk-abc", UUID: "UUID0000"}
	body := []byte("body-bytes")
	path := "/video-translation/translate"

	h := SecYaHeaders("Vtrans", session, body, path)

	if h["Vtrans-Signature"] != Signature(body) {
		t.Errorf("Vtrans-Signature mismatch")
	}
	if h["Sec-Vtrans-Sk"] != "sk-abc" {
		t.Errorf("Sec-Vtrans-Sk = %q", h["Sec-Vtrans-Sk"])
	}
	token := "UUID0000:" + path + ":" + config.ComponentVersion
	wantToken := Signature([]byte(token)) + ":" + token
	if h["Sec-Vtrans-Token"] != wantToken {
		t.Errorf("Sec-Vtrans-Token = %q, want %q", h["Sec-Vtrans-Token"], wantToken)
	}
}

func TestUUID(t *testing.T) {
	u := UUID()
	if len(u) != 32 {
		t.Fatalf("len(UUID) = %d, want 32", len(u))
	}
	if u != strings.ToUpper(u) {
		t.Errorf("UUID must be uppercase hex: %q", u)
	}
	for _, c := range u {
		if !strings.ContainsRune("0123456789ABCDEF", c) {
			t.Errorf("non-hex char %q in UUID", c)
		}
	}
	if UUID() == UUID() {
		t.Errorf("UUID should be random")
	}
}

func TestSessionValid(t *testing.T) {
	s := Session{SecretKey: "k", Expires: 100, Timestamp: 1000}
	if !s.Valid(1099) {
		t.Errorf("session should be valid before expiry")
	}
	if s.Valid(1100) {
		t.Errorf("session should be invalid at/after expiry")
	}
	if (Session{Timestamp: 1000, Expires: 100}).Valid(1000) {
		t.Errorf("session without secret key must be invalid")
	}
}
