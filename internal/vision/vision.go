package vision

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Source struct {
	URL  string `json:"url,omitempty"`
	Path string `json:"path,omitempty"`
	Data string `json:"data,omitempty"`
}

type Service struct {
	Client     *http.Client
	MaxImageMB int64
	FetchMaxMB int64
}

func (s Service) Load(source Source) (string, error) {
	if (source.URL == "") == (source.Path == "") == (source.Data == "") {
		return "", errors.New("provide exactly one image source: url, path, or data")
	}
	if source.Path != "" && !filepath.IsAbs(source.Path) && (filepath.Clean(source.Path) == ".." || strings.HasPrefix(filepath.Clean(source.Path), ".."+string(filepath.Separator))) {
		return "", errors.New("relative path must not escape the current working directory")
	}
	var data []byte
	var mediaType string
	switch {
	case source.URL != "":
		if err := validateHTTPURL(source.URL); err != nil {
			return "", err
		}
		var err error
		data, mediaType, err = s.fetch(source.URL)
		if err != nil {
			return "", err
		}
	case source.Path != "":
		file, err := os.Open(source.Path)
		if err != nil {
			return "", fmt.Errorf("open image: %w", err)
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil {
			return "", fmt.Errorf("stat image: %w", err)
		}
		if s.MaxImageMB > 0 && stat.Size() > s.MaxImageMB*1024*1024 {
			return "", fmt.Errorf("image exceeds %d MiB", s.MaxImageMB)
		}
		data, err = io.ReadAll(io.LimitReader(file, stat.Size()+1))
		if err != nil {
			return "", fmt.Errorf("read image: %w", err)
		}
		mediaType = http.DetectContentType(data)
	default:
		var err error
		data, err = base64.StdEncoding.DecodeString(source.Data)
		if err != nil {
			return "", fmt.Errorf("decode base64 image: %w", err)
		}
		mediaType = http.DetectContentType(data)
	}
	if len(data) == 0 {
		return "", errors.New("image is empty")
	}
	if s.MaxImageMB > 0 && int64(len(data)) > s.MaxImageMB*1024*1024 {
		return "", fmt.Errorf("image exceeds %d MiB", s.MaxImageMB)
	}
	encoded, format, err := normalizeImage(data)
	if err != nil {
		return "", fmt.Errorf("invalid or unsupported image: %w", err)
	}
	_ = mediaType
	return "data:image/" + format + ";base64," + encoded, nil
}

func validateHTTPURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse image URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("image URL scheme must be http or https")
	}
	if parsed.Hostname() == "" {
		return errors.New("image URL host is required")
	}
	if parsed.User != nil {
		return errors.New("image URL credentials are not allowed")
	}
	return nil
}

func (s Service) fetch(raw string) ([]byte, string, error) {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	limit := s.FetchMaxMB
	if limit <= 0 {
		limit = 20
	}
	resp, err := client.Get(raw)
	if err != nil {
		return nil, "", fmt.Errorf("fetch image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("fetch image: unexpected status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit*1024*1024+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image response: %w", err)
	}
	if int64(len(data)) > limit*1024*1024 {
		return nil, "", fmt.Errorf("image response exceeds %d MiB", limit)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func normalizeImage(data []byte) (string, string, error) {
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", "", err
	}
	var output bytes.Buffer
	if err := png.Encode(&output, decoded); err != nil {
		return "", "", fmt.Errorf("encode PNG: %w", err)
	}
	return base64.StdEncoding.EncodeToString(output.Bytes()), format, nil
}

func BlankPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		panic(err)
	}
	return output.Bytes()
}
