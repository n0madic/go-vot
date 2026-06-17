// Package secure implements the request signing used by the Yandex VOT API:
// HMAC-SHA256 over the request body and a per-request security token, plus the
// session UUID generator. Ported from @vot.js/shared/dist/secure.js.
package secure

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/n0madic/go-vot/pkg/config"
)

// Signature returns the lowercase hex HMAC-SHA256 of body keyed with the public
// VOT HMAC key.
func Signature(body []byte) string {
	mac := hmac.New(sha256.New, []byte(config.HMACKey))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Session is a per-module authenticated session returned by /session/create.
type Session struct {
	SecretKey string
	Expires   int32
	Timestamp int64 // unix seconds when the session was created
	UUID      string
}

// Valid reports whether the session is still valid at the given unix timestamp.
func (s Session) Valid(now int64) bool {
	return s.SecretKey != "" && s.Timestamp+int64(s.Expires) > now
}

// SecYaHeaders builds the per-request security headers. secType is "Vtrans" for
// translation/stream/audio requests and "Vsubs" for subtitle requests.
func SecYaHeaders(secType string, session Session, body []byte, path string) map[string]string {
	token := session.UUID + ":" + path + ":" + config.ComponentVersion
	tokenSign := Signature([]byte(token))
	return map[string]string{
		secType + "-Signature":      Signature(body),
		"Sec-" + secType + "-Sk":    session.SecretKey,
		"Sec-" + secType + "-Token": tokenSign + ":" + token,
	}
}

const hexDigits = "0123456789ABCDEF"

// UUID returns 32 random uppercase hex characters, matching the upstream getUUID
// (note: this is NOT an RFC 4122 UUID, just a 32-char hex token).
func UUID() string {
	rnd := make([]byte, 32)
	if _, err := rand.Read(rnd); err != nil {
		// crypto/rand failure is unrecoverable; panic mirrors a hard fault.
		panic("secure: cannot read random bytes: " + err.Error())
	}
	out := make([]byte, 32)
	for i := range out {
		out[i] = hexDigits[rnd[i]&0x0f]
	}
	return string(out)
}
