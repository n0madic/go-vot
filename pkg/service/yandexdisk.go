package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"regexp"
	"strings"
)

const yandexDiskInlinePrefix = "/i/"

var (
	yandexDiskSingleFileRe = regexp.MustCompile(`^/d/([^/]+)$`)
	yandexDiskExtRe        = regexp.MustCompile(`\.[^.]+$`)
)

// ydResource is one file/folder entry in a public Yandex.Disk listing.
type ydResource struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Hash string `json:"hash"`
	Meta struct {
		ShortURL      string  `json:"short_url"`
		MediaType     string  `json:"mediatype"`
		VideoDuration float64 `json:"videoDuration"`
	} `json:"meta"`
}

func yandexDiskID(_ context.Context, _ Fetcher, u *url.URL) (string, error) {
	if fileID := reFind(`/i/([^/]+)`, u.Path, 0); fileID != "" {
		return fileID, nil
	}
	if reFind(`/d/([^/]+)`, u.Path, 0) != "" {
		return u.Path, nil
	}
	return "", nil
}

// yandexDiskData resolves a public Yandex.Disk video. A direct file link (/i/ or
// a single /d/<hash>) maps straight to the public viewer URL; a nested folder
// path is resolved through the public listing API. On any failure it falls back
// to the clear viewer link. Ported from @vot.js/node/helpers/yandexdisk.js.
func yandexDiskData(ctx context.Context, f Fetcher, svc *Service, origin, videoID string) (*VideoData, error) {
	if strings.HasPrefix(videoID, yandexDiskInlinePrefix) || yandexDiskSingleFileRe.MatchString(videoID) {
		return &VideoData{URL: svc.BaseURL + videoID[1:]}, nil
	}
	if decoded, err := url.PathUnescape(videoID); err == nil {
		videoID = decoded
	}
	return yandexDiskDiskData(ctx, f, svc, origin, videoID), nil
}

func yandexDiskDiskData(ctx context.Context, f Fetcher, svc *Service, origin, videoID string) *VideoData {
	fallback := &VideoData{URL: svc.BaseURL + strings.TrimPrefix(videoID, "/")}

	apiOrigin := origin
	if apiOrigin == "" {
		apiOrigin = "https://disk.yandex.ru"
	}

	body, err := getText(ctx, f, apiOrigin+videoID, map[string]string{
		"Origin":  apiOrigin,
		"Referer": apiOrigin + "/client/disk",
	})
	if err != nil {
		return fallback
	}

	raw := reFind(`(?s)<script[^>]*id="store-prefetch"[^>]*>(.*?)</script>`, body, 1)
	if raw == "" {
		return fallback
	}
	var prefetch struct {
		Resources      map[string]ydResource `json:"resources"`
		RootResourceID string                `json:"rootResourceId"`
		Environment    struct {
			SK string `json:"sk"`
		} `json:"environment"`
	}
	if err := json.Unmarshal([]byte(raw), &prefetch); err != nil {
		return fallback
	}

	resourcePaths := strings.Split(videoID, "/")
	if len(resourcePaths) <= 3 {
		return fallback
	}
	resourcePaths = resourcePaths[3:]
	last := resourcePaths[len(resourcePaths)-1]
	resourcePath := strings.Join(resourcePaths[:len(resourcePaths)-1], "/")

	resourcesList := make([]ydResource, 0, len(prefetch.Resources))
	for _, r := range prefetch.Resources {
		resourcesList = append(resourcesList, r)
	}
	if strings.Contains(resourcePath, "/") {
		rootResource := prefetch.Resources[prefetch.RootResourceID]
		list, err := yandexDiskFetchList(ctx, f, apiOrigin, rootResource.Hash+":/"+resourcePath, prefetch.Environment.SK)
		if err != nil {
			return fallback
		}
		resourcesList = list
	}

	var resource *ydResource
	for i := range resourcesList {
		if resourcesList[i].Name == last {
			resource = &resourcesList[i]
			break
		}
	}
	if resource == nil || resource.Type == "dir" || resource.Meta.MediaType != "video" {
		return fallback
	}

	out := &VideoData{
		Title:    yandexDiskClearTitle(resource.Name),
		Duration: math.Round(resource.Meta.VideoDuration / 1000),
	}
	if resource.Meta.ShortURL != "" {
		out.URL = resource.Meta.ShortURL
		return out
	}
	downloadURL, err := yandexDiskDownloadURL(ctx, f, apiOrigin, resource.Path, prefetch.Environment.SK)
	if err != nil {
		return fallback
	}
	out.URL = downloadURL
	return out
}

// yandexDiskBodyHash builds the URL-encoded {hash, sk} body the listing API expects.
func yandexDiskBodyHash(hash, sk string) ([]byte, error) {
	data, err := json.Marshal(map[string]string{"hash": hash, "sk": sk})
	if err != nil {
		return nil, err
	}
	return []byte(url.QueryEscape(string(data))), nil
}

func yandexDiskClearTitle(name string) string {
	return yandexDiskExtRe.ReplaceAllString(name, "")
}

func yandexDiskFetchList(ctx context.Context, f Fetcher, apiOrigin, dirHash, sk string) ([]ydResource, error) {
	var resp struct {
		Resources []ydResource `json:"resources"`
		Error     any          `json:"error"`
	}
	body, err := yandexDiskBodyHash(dirHash, sk)
	if err != nil {
		return nil, err
	}
	if err := postJSON(ctx, f, apiOrigin+"/public/api/fetch-list", body, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, errors.New("yandexdisk: failed to fetch folder list")
	}
	return resp.Resources, nil
}

func yandexDiskDownloadURL(ctx context.Context, f Fetcher, apiOrigin, fileHash, sk string) (string, error) {
	var resp struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
		Error any `json:"error"`
	}
	body, err := yandexDiskBodyHash(fileHash, sk)
	if err != nil {
		return "", err
	}
	if err := postJSON(ctx, f, apiOrigin+"/public/api/download-url", body, nil, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil || resp.Data.URL == "" {
		return "", errors.New("yandexdisk: failed to get download url")
	}
	return resp.Data.URL, nil
}
