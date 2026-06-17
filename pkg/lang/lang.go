// Package lang exposes the languages supported by the Yandex VOT API and a
// small ISO 639-2 -> 639-1 mapping, ported from @vot.js/shared.
package lang

import "slices"

// AvailableLangs are the languages accepted as the source (request) language.
// "auto" asks Yandex to auto-detect the source language.
var AvailableLangs = []string{
	"auto", "ru", "en", "zh", "ko", "lt", "lv", "ar", "fr", "it", "es", "de", "ja",
}

// AvailableTTS are the languages the translation can be voiced into
// (the response/target language).
var AvailableTTS = []string{"ru", "en", "kk"}

// SubtitlesFormats lists the subtitle output formats supported by the converter.
var SubtitlesFormats = []string{"srt", "vtt", "json"}

// IsAvailableLang reports whether lang is a valid source language.
func IsAvailableLang(lang string) bool {
	return slices.Contains(AvailableLangs, lang)
}

// IsAvailableTTS reports whether lang is a valid target (TTS) language.
func IsAvailableTTS(lang string) bool {
	return slices.Contains(AvailableTTS, lang)
}

// iso6392to6391 maps three-letter ISO 639-2 codes to two-letter ISO 639-1 codes.
// Ported from @vot.js/shared/utils/utils.js (used when a service reports the
// source language in the three-letter form).
var iso6392to6391 = map[string]string{
	"afr": "af", "aka": "ak", "alb": "sq", "amh": "am", "ara": "ar", "arm": "hy",
	"asm": "as", "aym": "ay", "aze": "az", "baq": "eu", "bel": "be", "ben": "bn",
	"bos": "bs", "bul": "bg", "bur": "my", "cat": "ca", "chi": "zh", "cos": "co",
	"cze": "cs", "dan": "da", "div": "dv", "dut": "nl", "eng": "en", "epo": "eo",
	"est": "et", "ewe": "ee", "fin": "fi", "fre": "fr", "fry": "fy", "geo": "ka",
	"ger": "de", "gla": "gd", "gle": "ga", "glg": "gl", "gre": "el", "grn": "gn",
	"guj": "gu", "hat": "ht", "hau": "ha", "hin": "hi", "hrv": "hr", "hun": "hu",
	"ibo": "ig", "ice": "is", "ind": "id", "ita": "it", "jav": "jv", "jpn": "ja",
	"kan": "kn", "kaz": "kk", "khm": "km", "kin": "rw", "kir": "ky", "kor": "ko",
	"kur": "ku", "lao": "lo", "lat": "la", "lav": "lv", "lin": "ln", "lit": "lt",
	"ltz": "lb", "lug": "lg", "mac": "mk", "mal": "ml", "mao": "mi", "mar": "mr",
	"may": "ms", "mlg": "mg", "mlt": "mt", "mon": "mn", "nep": "ne", "nor": "no",
	"nya": "ny", "ori": "or", "orm": "om", "pan": "pa", "per": "fa", "pol": "pl",
	"por": "pt", "pus": "ps", "rum": "ro", "run": "rn", "rus": "ru", "sin": "si",
	"slo": "sk", "slv": "sl", "sme": "se", "smo": "sm", "sna": "sn", "snd": "sd",
	"som": "so", "spa": "es", "sun": "su", "swa": "sw", "swe": "sv", "tam": "ta",
	"tat": "tt", "tel": "te", "tgk": "tg", "tgl": "tl", "tha": "th", "tir": "ti",
	"tuk": "tk", "tur": "tr", "uig": "ug", "ukr": "uk", "urd": "ur", "uzb": "uz",
	"vie": "vi", "wel": "cy", "wol": "wo", "xho": "xh", "yid": "yi", "yor": "yo",
	"zul": "zu",
}

// Normalize converts a three-letter ISO 639-2 code to its two-letter ISO 639-1
// equivalent. Codes that are already two letters (or unknown) are returned as-is.
func Normalize(code string) string {
	if v, ok := iso6392to6391[code]; ok {
		return v
	}
	return code
}

// iso6391to6392 is the reverse of iso6392to6391 (2-letter -> 3-letter ISO 639-2).
var iso6391to6392 = func() map[string]string {
	m := make(map[string]string, len(iso6392to6391))
	for three, two := range iso6392to6391 {
		if _, ok := m[two]; !ok {
			m[two] = three
		}
	}
	return m
}()

// ISO6392 returns the three-letter ISO 639-2 code for a two-letter ISO 639-1
// code (e.g. "ru" -> "rus"), as expected by container language metadata.
// Three-letter or unknown codes are returned unchanged.
func ISO6392(code string) string {
	if len(code) == 3 {
		return code
	}
	if v, ok := iso6391to6392[code]; ok {
		return v
	}
	return code
}
