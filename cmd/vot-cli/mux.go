package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// muxOptions configures how the translated audio is combined with the source.
type muxOptions struct {
	// OrigVolume is the level of the original audio in the mix (0..1): a constant
	// reduction in "classic" mode, the in-pauses baseline in "smart" mode.
	OrigVolume float64
	// Smart enables adaptive ducking (sidechaincompress) instead of a constant mix.
	Smart bool
	// TransLang / OrigLang are ISO 639-2 codes used as audio-track language tags
	// (empty = no tag).
	TransLang string
	OrigLang  string
}

// buildMuxFilter builds the ffmpeg filter_complex graph. Both inputs are split
// because each is used twice: the original feeds the mix and a standalone track,
// the translation feeds the mix (and, in smart mode, the sidechain trigger).
//
//   - classic: original attenuated to OrigVolume and amix-ed with the translation;
//   - smart: original kept at the OrigVolume baseline, then ducked by
//     sidechaincompress whenever the translation is speaking, then amix-ed.
func buildMuxFilter(opts muxOptions) string {
	vol := strconv.FormatFloat(opts.OrigVolume, 'f', 3, 64)
	if opts.Smart {
		return "[1:a]asplit=2[sc][tr];[0:a]asplit=2[am][ao];" +
			"[am]volume=" + vol + "[base];" +
			"[base][sc]sidechaincompress=threshold=0.03:ratio=12:attack=20:release=300[duck];" +
			"[duck][tr]amix=inputs=2:duration=longest:normalize=0[mix]"
	}
	return "[0:a]asplit=2[am][ao];" +
		"[am]volume=" + vol + "[orig];" +
		"[orig][1:a]amix=inputs=2:duration=longest[mix]"
}

// muxAudio combines a source video with the translated audio using ffmpeg.
//
// It produces two audio tracks, mirroring the Yandex voice-over experience:
//   - track 0 (default): the translation over the original at reduced volume
//     (constant in classic mode, dynamically ducked during speech in smart mode);
//   - track 1: the untouched original audio, so the viewer can switch back.
//
// The original video stream is copied; audio is re-encoded to AAC.
func muxAudio(ctx context.Context, videoSource, audioPath, outPath string, opts muxOptions) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-stats",
		"-y",
		"-i", videoSource,
		"-i", audioPath,
		"-filter_complex", buildMuxFilter(opts),
		"-map", "0:v",
		"-map", "[mix]",
		"-map", "[ao]",
		"-c:v", "copy",
		"-c:a", "aac",
	}
	if opts.TransLang != "" {
		args = append(args, "-metadata:s:a:0", "language="+opts.TransLang)
	}
	if opts.OrigLang != "" {
		args = append(args, "-metadata:s:a:1", "language="+opts.OrigLang)
	}
	args = append(args, outPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg mux failed: %w", err)
	}
	return nil
}
