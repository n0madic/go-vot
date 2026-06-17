package subs

import (
	"strings"
	"testing"
)

func TestMsToStr(t *testing.T) {
	if got := msToStr(3661500, ","); got != "01:01:01,500" {
		t.Errorf("msToStr = %q", got)
	}
	if got := msToStr(0, "."); got != "00:00:00.000" {
		t.Errorf("msToStr = %q", got)
	}
}

func TestStrToMs(t *testing.T) {
	if got := strToMs("01:01:01,500"); got != 3661500 {
		t.Errorf("strToMs = %d, want 3661500", got)
	}
	if got := strToMs("00:00:02.500"); got != 2500 {
		t.Errorf("strToMs = %d, want 2500", got)
	}
}

func TestJSONToSRT(t *testing.T) {
	d := Data{Subtitles: []Subtitle{
		{Text: "Hello", StartMs: 0, DurationMs: 1000},
		{Text: "World", StartMs: 1000, DurationMs: 1500},
	}}
	srt := FromJSON(d, "srt")
	want := "1\n00:00:00,000 --> 00:00:01,000\nHello\n\n2\n00:00:01,000 --> 00:00:02,500\nWorld"
	if srt != want {
		t.Errorf("srt =\n%q\nwant\n%q", srt, want)
	}
}

func TestJSONToVTT(t *testing.T) {
	d := Data{Subtitles: []Subtitle{{Text: "Hi", StartMs: 500, DurationMs: 500}}}
	vtt := FromJSON(d, "vtt")
	if !strings.HasPrefix(vtt, "WEBVTT\n\n") {
		t.Errorf("vtt missing header: %q", vtt)
	}
	if !strings.Contains(vtt, "00:00:00.500 --> 00:00:01.000\nHi") {
		t.Errorf("vtt body wrong: %q", vtt)
	}
}

func TestSRTRoundTrip(t *testing.T) {
	d := Data{Subtitles: []Subtitle{
		{Text: "Hello", StartMs: 0, DurationMs: 1000},
		{Text: "World", StartMs: 2000, DurationMs: 1500},
	}}
	srt := FromJSON(d, "srt")
	back := ToJSON(srt, "srt")
	if len(back.Subtitles) != 2 {
		t.Fatalf("got %d subtitles", len(back.Subtitles))
	}
	if back.Subtitles[0].Text != "Hello" || back.Subtitles[0].StartMs != 0 || back.Subtitles[0].DurationMs != 1000 {
		t.Errorf("sub0 = %+v", back.Subtitles[0])
	}
	if back.Subtitles[1].StartMs != 2000 || back.Subtitles[1].DurationMs != 1500 {
		t.Errorf("sub1 = %+v", back.Subtitles[1])
	}
}

func TestConvert(t *testing.T) {
	jsonData := []byte(`{"containsTokens":false,"subtitles":[{"text":"Hi","startMs":0,"durationMs":1000}]}`)

	if DetectFormat(jsonData) != "json" {
		t.Error("DetectFormat json failed")
	}

	srt, err := Convert(jsonData, "srt")
	if err != nil {
		t.Fatal(err)
	}
	if DetectFormat(srt) != "srt" {
		t.Errorf("converted output not srt: %s", srt)
	}
	if !strings.Contains(string(srt), "00:00:00,000 --> 00:00:01,000") {
		t.Errorf("srt = %s", srt)
	}

	// Same format in/out returns unchanged.
	same, _ := Convert(srt, "srt")
	if string(same) != string(srt) {
		t.Error("same-format conversion should be a no-op")
	}
}
