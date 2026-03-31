package modernimage

import (
	"os"
	"path/filepath"
	"testing"
)

func loadTestData(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatalf("failed to load test data %s: %v", name, err)
	}
	return data
}

func isWebP(data []byte) bool {
	return len(data) >= 12 &&
		data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
		data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P'
}

func isAVIF(data []byte) bool {
	return len(data) >= 12 &&
		data[4] == 'f' && data[5] == 't' && data[6] == 'y' && data[7] == 'p'
}

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatal("Version() returned empty string")
	}
	t.Logf("libmodernimage version: %s", v)
}

func TestWebpEncodeLossyJPEG(t *testing.T) {
	data := loadTestData(t, "photo.jpg")
	result, err := Webp.EncodeLossy(data, 80, false)
	if err != nil {
		t.Fatalf("Webp.EncodeLossy failed: %v", err)
	}
	if !isWebP(result.Data) {
		t.Fatal("output is not WebP")
	}
	if result.MimeType != "image/webp" {
		t.Fatalf("expected mime image/webp, got %s", result.MimeType)
	}
	t.Logf("JPEG -> WebP lossy: %d -> %d bytes", len(data), len(result.Data))
}

func TestWebpEncodeLossyPNG(t *testing.T) {
	data := loadTestData(t, "logo.png")
	result, err := Webp.EncodeLossy(data, 80, true)
	if err != nil {
		t.Fatalf("Webp.EncodeLossy failed: %v", err)
	}
	if !isWebP(result.Data) {
		t.Fatal("output is not WebP")
	}
	t.Logf("PNG -> WebP lossy: %d -> %d bytes", len(data), len(result.Data))
}

func TestWebpEncodeLosslessPNG(t *testing.T) {
	data := loadTestData(t, "logo.png")
	result, err := Webp.EncodeLossless(data, false)
	if err != nil {
		t.Fatalf("Webp.EncodeLossless failed: %v", err)
	}
	if !isWebP(result.Data) {
		t.Fatal("output is not WebP")
	}
	t.Logf("PNG -> WebP lossless: %d -> %d bytes", len(data), len(result.Data))
}

func TestWebpEncodeLosslessJPEG(t *testing.T) {
	data := loadTestData(t, "photo.jpg")
	result, err := Webp.EncodeLossless(data, true)
	if err != nil {
		t.Fatalf("Webp.EncodeLossless failed: %v", err)
	}
	if !isWebP(result.Data) {
		t.Fatal("output is not WebP")
	}
	t.Logf("JPEG -> WebP lossless: %d -> %d bytes", len(data), len(result.Data))
}

func TestWebpEncodeGif(t *testing.T) {
	data := loadTestData(t, "animation.gif")
	result, err := Webp.EncodeGif(data, false)
	if err != nil {
		t.Fatalf("Webp.EncodeGif failed: %v", err)
	}
	if !isWebP(result.Data) {
		t.Fatal("output is not WebP")
	}
	t.Logf("GIF -> WebP: %d -> %d bytes", len(data), len(result.Data))
}

func TestAvifEncodeBalancedJPEG(t *testing.T) {
	data := loadTestData(t, "photo.jpg")
	result, err := Avif.EncodeBalanced(data, 80, 0)
	if err != nil {
		t.Fatalf("Avif.EncodeBalanced failed: %v", err)
	}
	if !isAVIF(result.Data) {
		t.Fatal("output is not AVIF")
	}
	if result.MimeType != "image/avif" {
		t.Fatalf("expected mime image/avif, got %s", result.MimeType)
	}
	t.Logf("JPEG -> AVIF balanced: %d -> %d bytes", len(data), len(result.Data))
}

func TestAvifEncodeBalancedPNG(t *testing.T) {
	data := loadTestData(t, "logo.png")
	result, err := Avif.EncodeBalanced(data, 80, 0)
	if err != nil {
		t.Fatalf("Avif.EncodeBalanced failed: %v", err)
	}
	if !isAVIF(result.Data) {
		t.Fatal("output is not AVIF")
	}
	t.Logf("PNG -> AVIF balanced: %d -> %d bytes", len(data), len(result.Data))
}

func TestAvifEncodeCompactJPEG(t *testing.T) {
	data := loadTestData(t, "photo.jpg")
	result, err := Avif.EncodeCompact(data, 80, 0)
	if err != nil {
		t.Fatalf("Avif.EncodeCompact failed: %v", err)
	}
	if !isAVIF(result.Data) {
		t.Fatal("output is not AVIF")
	}
	t.Logf("JPEG -> AVIF compact: %d -> %d bytes", len(data), len(result.Data))
}

func TestAvifEncodeFastJPEG(t *testing.T) {
	data := loadTestData(t, "photo.jpg")
	result, err := Avif.EncodeFast(data, 80, 0)
	if err != nil {
		t.Fatalf("Avif.EncodeFast failed: %v", err)
	}
	if !isAVIF(result.Data) {
		t.Fatal("output is not AVIF")
	}
	t.Logf("JPEG -> AVIF fast: %d -> %d bytes", len(data), len(result.Data))
}

func TestErrorEmptyInput(t *testing.T) {
	_, err := Webp.EncodeLossy(nil, 80, false)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestErrorWrongFormatGifToLossy(t *testing.T) {
	data := loadTestData(t, "animation.gif")
	_, err := Webp.EncodeLossy(data, 80, false)
	if err == nil {
		t.Fatal("expected error for GIF input to EncodeLossy")
	}
}

func TestErrorJpegToEncodeGif(t *testing.T) {
	data := loadTestData(t, "photo.jpg")
	_, err := Webp.EncodeGif(data, false)
	if err == nil {
		t.Fatal("expected error for JPEG input to EncodeGif")
	}
}
