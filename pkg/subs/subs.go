// Package subs converts subtitles between Yandex JSON, SRT and VTT formats.
// Ported from @vot.js/shared/dist/utils/subs.js.
package subs

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Subtitle is one cue in the Yandex JSON subtitle format. Timings are floats
// because the Yandex API returns them as JSON numbers like 18540.0.
type Subtitle struct {
	Text       string  `json:"text"`
	StartMs    float64 `json:"startMs"`
	DurationMs float64 `json:"durationMs"`
	SpeakerID  string  `json:"speakerId,omitempty"`
}

// Data is the Yandex JSON subtitle document.
type Data struct {
	ContainsTokens bool       `json:"containsTokens"`
	Subtitles      []Subtitle `json:"subtitles"`
}

var (
	blankLineRe = regexp.MustCompile(`\r?\n\r?\n`)
	srtIndexRe  = regexp.MustCompile(`^\d+\r?\n`)
	webvttRe    = regexp.MustCompile(`^WEBVTT`)
)

// msToStr formats milliseconds as HH:MM:SS<delim>mmm.
func msToStr(ms int, delim string) string {
	if ms < 0 {
		ms = 0
	}
	sec := ms / 1000
	hours := sec / 3600
	minutes := (sec % 3600) / 60
	remSec := sec % 60
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d%s%03d", hours, minutes, remSec, delim, millis)
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// strToMs parses an SRT/VTT timestamp (HH:MM:SS,mmm or MM:SS.mmm) to milliseconds.
func strToMs(t string) int {
	t = strings.SplitN(t, " ", 2)[0]
	parts := strings.Split(t, ":")
	for len(parts) < 3 {
		parts = append([]string{"00"}, parts...)
	}
	strHours, strMinutes, strSeconds := parts[0], parts[1], parts[2]
	// The upstream removes the first ,/. so "02,500" -> 2500 (already in ms).
	strSeconds = strings.Replace(strSeconds, ",", "", 1)
	strSeconds = strings.Replace(strSeconds, ".", "", 1)
	return atoiSafe(strHours)*3600000 + atoiSafe(strMinutes)*60000 + atoiSafe(strSeconds)
}

// FromJSON renders the JSON subtitle document as SRT (output="srt") or VTT.
func FromJSON(d Data, output string) string {
	isVTT := output == "vtt"
	delim := ","
	if isVTT {
		delim = "."
	}
	var b strings.Builder
	for i, s := range d.Subtitles {
		if !isVTT {
			fmt.Fprintf(&b, "%d\n", i+1)
		}
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", msToStr(int(s.StartMs), delim), msToStr(int(s.StartMs+s.DurationMs), delim), s.Text)
	}
	out := strings.TrimSpace(b.String())
	if isVTT {
		return "WEBVTT\n\n" + out
	}
	return out
}

// ToJSON parses SRT/VTT text into the JSON subtitle document.
func ToJSON(data, from string) Data {
	parts := blankLineRe.Split(data, -1)
	if from == "vtt" && len(parts) > 0 {
		parts = parts[1:] // drop the WEBVTT header block
	}
	if len(parts) > 0 && srtIndexRe.MatchString(parts[0]) {
		from = "srt"
	}
	offset := 0
	if from == "srt" {
		offset = 1
	}

	var subs []Subtitle
	for _, part := range parts {
		lines := strings.Split(strings.TrimSpace(part), "\n")
		timeLine := ""
		if offset < len(lines) {
			timeLine = lines[offset]
		}
		text := ""
		if offset+1 < len(lines) {
			text = strings.Join(lines[offset+1:], "\n")
		}
		if (len(lines) != 2 || !strings.Contains(part, " --> ")) && !strings.Contains(timeLine, " --> ") {
			if len(subs) == 0 {
				continue
			}
			subs[len(subs)-1].Text += "\n\n" + strings.Join(lines, "\n")
			continue
		}
		st := strings.SplitN(timeLine, " --> ", 2)
		if len(st) != 2 {
			continue
		}
		startMs := strToMs(st[0])
		endMs := strToMs(st[1])
		subs = append(subs, Subtitle{
			Text:       text,
			StartMs:    float64(startMs),
			DurationMs: float64(endMs - startMs),
			SpeakerID:  "0",
		})
	}
	return Data{ContainsTokens: false, Subtitles: subs}
}

// DetectFormat reports the format of raw subtitle bytes: "json", "vtt" or "srt".
func DetectFormat(raw []byte) string {
	s := strings.TrimLeft(string(raw), " \t\r\n")
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return "json"
	}
	if webvttRe.MatchString(s) {
		return "vtt"
	}
	return "srt"
}

// Convert converts raw subtitle bytes to the requested output format
// ("srt", "vtt" or "json"). When the input is already in the target format it is
// returned unchanged.
func Convert(raw []byte, output string) ([]byte, error) {
	from := DetectFormat(raw)
	if from == output {
		return raw, nil
	}
	if from == "json" {
		var d Data
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("parse json subtitles: %w", err)
		}
		return []byte(FromJSON(d, output)), nil
	}
	d := ToJSON(string(raw), from)
	if output == "json" {
		return json.Marshal(d)
	}
	return []byte(FromJSON(d, output)), nil
}
