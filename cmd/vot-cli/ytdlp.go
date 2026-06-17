package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ytdlpAvailable reports whether the yt-dlp binary is in PATH.
func ytdlpAvailable() bool {
	_, err := exec.LookPath("yt-dlp")
	return err == nil
}

// ytdlpFormat builds the yt-dlp -f selector for a maximum video height. maxHeight
// 0 means "best" (no cap). With a cap it prefers streams at or below the height
// and falls back to the best available if none qualify.
func ytdlpFormat(maxHeight int) string {
	if maxHeight <= 0 {
		return "bv*+ba/b"
	}
	return fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]/bv*+ba/b", maxHeight, maxHeight)
}

// ytdlpDownload fetches the source video with yt-dlp into outDir using baseName
// (the extension is chosen by yt-dlp) and returns the path to the downloaded
// file. It selects video+audio capped at maxHeight (0 = best) and merges with
// ffmpeg.
func ytdlpDownload(ctx context.Context, link, outDir, baseName string, maxHeight int) (string, error) {
	outTmpl := filepath.Join(outDir, baseName+".%(ext)s")
	args := []string{
		"-f", ytdlpFormat(maxHeight),
		"--no-playlist",
		"--retries", "10",
		"--fragment-retries", "10",
		"-o", outTmpl,
		"--no-simulate",
		"--print", "after_move:filepath",
		link,
	}
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("yt-dlp: %w", err)
	}

	if path := lastNonEmptyLine(stdout.String()); path != "" {
		return path, nil
	}
	// Fallback: locate the file we asked yt-dlp to write.
	if matches, _ := filepath.Glob(filepath.Join(outDir, baseName+".*")); len(matches) > 0 {
		return matches[0], nil
	}
	return "", fmt.Errorf("yt-dlp did not report an output file")
}

// ytdlpTitle returns the video title via yt-dlp without downloading anything.
// Returns "" if yt-dlp is unavailable or the lookup fails.
func ytdlpTitle(ctx context.Context, link string) string {
	if !ytdlpAvailable() {
		return ""
	}
	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--skip-download", "--no-playlist", "--quiet", "--no-warnings",
		"--print", "title", link)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return lastNonEmptyLine(out.String())
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
