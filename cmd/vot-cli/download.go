package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/n0madic/go-vot/pkg/config"
)

// downloadFile streams rawURL to dest and returns the number of bytes written.
func downloadFile(ctx context.Context, hc *http.Client, rawURL, dest string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", config.UserAgent)

	resp, err := hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return 0, err
	}

	n, err := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		// Don't leave a truncated file behind for a later mux step to pick up.
		os.Remove(dest)
		return n, fmt.Errorf("write %s: %w", dest, err)
	}
	if closeErr != nil {
		os.Remove(dest)
		return n, fmt.Errorf("close %s: %w", dest, closeErr)
	}
	return n, nil
}

// fetchBytes downloads rawURL into memory (used for subtitles).
func fetchBytes(ctx context.Context, hc *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", config.UserAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", rawURL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
