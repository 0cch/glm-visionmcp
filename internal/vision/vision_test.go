package vision

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/gif"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBase64AndNormalize(t *testing.T) {
	service := Service{MaxImageMB: 1}
	dataURL, err := service.Load(Source{Data: base64.StdEncoding.EncodeToString(BlankPNG())})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !strings.HasPrefix(dataURL, "data:image/png;base64,") {
		t.Fatalf("Load() = %q", dataURL)
	}
}

func TestLoadJPEGAndGIF(t *testing.T) {
	service := Service{MaxImageMB: 1}
	for name, image := range map[string][]byte{
		"jpeg": BlankJPEG(),
		"gif":  BlankGIF(),
	} {
		dataURL, err := service.Load(Source{Data: base64.StdEncoding.EncodeToString(image)})
		if err != nil {
			t.Fatalf("Load(%s) error = %v", name, err)
		}
		if !strings.HasPrefix(dataURL, "data:image/"+name+";base64,") {
			t.Fatalf("Load(%s) = %q", name, dataURL)
		}
	}
}

func TestLoadRequiresExactlyOneSource(t *testing.T) {
	service := Service{}
	if _, err := service.Load(Source{}); err == nil || !strings.Contains(err.Error(), "exactly one image source") {
		t.Fatalf("Load() error = %v", err)
	}
	if _, err := service.Load(Source{URL: "https://example.com/a.png", Path: "a.png"}); err == nil {
		t.Fatal("expected two-source error")
	}
}

func TestLoadRejectsEscapingPath(t *testing.T) {
	service := Service{}
	if _, err := service.Load(Source{Path: `..\secret.png`}); err == nil || !strings.Contains(err.Error(), "must not escape") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadAbsoluteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "image.png")
	if err := os.WriteFile(path, BlankPNG(), 0o600); err != nil {
		t.Fatal(err)
	}
	service := Service{MaxImageMB: 1}
	if _, err := service.Load(Source{Path: path}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRelativeFile(t *testing.T) {
	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWD)
	if err := os.WriteFile("image.png", BlankPNG(), 0o600); err != nil {
		t.Fatal(err)
	}
	service := Service{MaxImageMB: 1}
	if _, err := service.Load(Source{Path: filepath.Join("subdir", "..", "image.png")}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadURLRejectsCredentialsAndFetchesOnce(t *testing.T) {
	service := Service{MaxImageMB: 1}
	if _, err := service.Load(Source{URL: "https://user:pass@example.com/a.png"}); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("Load() error = %v", err)
	}
	if _, err := service.Load(Source{URL: "ftp://example.com/a.png"}); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadHTTPSuccessAndFailure(t *testing.T) {
	success := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/image.png" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write(BlankPNG()); err != nil {
			t.Errorf("Write() error = %v", err)
		}
	}))
	defer success.Close()
	service := Service{MaxImageMB: 1}
	if _, err := service.Load(Source{URL: success.URL + "/image.png"}); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, err := service.Load(Source{URL: success.URL + "/missing.png"}); err == nil || !strings.Contains(err.Error(), "unexpected status 404") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsInvalidImage(t *testing.T) {
	service := Service{}
	if _, err := service.Load(Source{Data: base64.StdEncoding.EncodeToString([]byte("not image"))}); err == nil {
		t.Fatal("expected invalid image error")
	}
}

func BlankJPEG() []byte {
	var output bytes.Buffer
	if err := jpeg.Encode(&output, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		panic(err)
	}
	return output.Bytes()
}

func BlankGIF() []byte {
	var output bytes.Buffer
	if err := gif.Encode(&output, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		panic(err)
	}
	return output.Bytes()
}
