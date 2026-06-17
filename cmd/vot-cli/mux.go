package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// muxAudio combines a source video with the translated audio track using ffmpeg.
// The original video stream is copied; the translated audio is re-encoded to AAC.
// When audioLang (an ISO 639-2 code) is non-empty it is tagged on the audio stream.
func muxAudio(ctx context.Context, videoSource, audioPath, outPath, audioLang string) error {
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
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-c:v", "copy",
		"-c:a", "aac",
	}
	if audioLang != "" {
		args = append(args, "-metadata:s:a:0", "language="+audioLang)
	}
	args = append(args, "-shortest", outPath)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg mux failed: %w", err)
	}
	return nil
}
