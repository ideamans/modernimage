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

// --- WebP Lossy: JPEG inputs ---

var jpegFiles = []string{
	"photo.jpg",
	"photo-like.jpg",
	"landscape-like.jpg",
	"medium-512x512.jpg",
	"edges.jpg",
	"gradient-radial.jpg",
	"small-128x128.jpg",
}

func TestWebpEncodeLossyJPEG(t *testing.T) {
	for _, name := range jpegFiles {
		t.Run(name, func(t *testing.T) {
			data := loadTestData(t, name)
			result, err := Webp.EncodeLossy(data, 80, false)
			if err != nil {
				t.Fatalf("Webp.EncodeLossy(%s) failed: %v", name, err)
			}
			if !isWebP(result.Data) {
				t.Fatal("output is not WebP")
			}
			t.Logf("%s: %d -> %d bytes", name, len(data), len(result.Data))
		})
	}
}

// --- WebP Lossy: PNG inputs ---

var pngFiles = []string{
	"logo.png",
	"photo-like.png",
	"text.png",
	"flat-color.png",
	"gradient-horizontal.png",
	"small-128x128.png",
}

func TestWebpEncodeLossyPNG(t *testing.T) {
	for _, name := range pngFiles {
		t.Run(name, func(t *testing.T) {
			data := loadTestData(t, name)
			result, err := Webp.EncodeLossy(data, 80, true)
			if err != nil {
				t.Fatalf("Webp.EncodeLossy(%s) failed: %v", name, err)
			}
			if !isWebP(result.Data) {
				t.Fatal("output is not WebP")
			}
			t.Logf("%s: %d -> %d bytes", name, len(data), len(result.Data))
		})
	}
}

// --- WebP Lossless ---

func TestWebpEncodeLossless(t *testing.T) {
	files := append(jpegFiles, pngFiles...)
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			data := loadTestData(t, name)
			result, err := Webp.EncodeLossless(data, false)
			if err != nil {
				t.Fatalf("Webp.EncodeLossless(%s) failed: %v", name, err)
			}
			if !isWebP(result.Data) {
				t.Fatal("output is not WebP")
			}
			t.Logf("%s: %d -> %d bytes", name, len(data), len(result.Data))
		})
	}
}

// --- WebP GIF ---

var gifFiles = []string{
	"animation.gif",
	"animated-3frames.gif",
	"animated-small.gif",
	"static-512x512.gif",
	"static-alpha.gif",
}

func TestWebpEncodeGif(t *testing.T) {
	for _, name := range gifFiles {
		t.Run(name, func(t *testing.T) {
			data := loadTestData(t, name)
			result, err := Webp.EncodeGif(data, false)
			if err != nil {
				t.Fatalf("Webp.EncodeGif(%s) failed: %v", name, err)
			}
			if !isWebP(result.Data) {
				t.Fatal("output is not WebP")
			}
			t.Logf("%s: %d -> %d bytes", name, len(data), len(result.Data))
		})
	}
}

// --- AVIF Balanced ---

func TestAvifEncodeBalanced(t *testing.T) {
	files := append(jpegFiles, pngFiles...)
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			data := loadTestData(t, name)
			result, err := Avif.EncodeBalanced(data, 80, 0)
			if err != nil {
				t.Fatalf("Avif.EncodeBalanced(%s) failed: %v", name, err)
			}
			if !isAVIF(result.Data) {
				t.Fatal("output is not AVIF")
			}
			t.Logf("%s: %d -> %d bytes", name, len(data), len(result.Data))
		})
	}
}

// --- AVIF Compact ---

func TestAvifEncodeCompact(t *testing.T) {
	// Use smaller files only (compact is slow)
	files := []string{"photo.jpg", "small-128x128.jpg", "logo.png", "small-128x128.png"}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			data := loadTestData(t, name)
			result, err := Avif.EncodeCompact(data, 80, 0)
			if err != nil {
				t.Fatalf("Avif.EncodeCompact(%s) failed: %v", name, err)
			}
			if !isAVIF(result.Data) {
				t.Fatal("output is not AVIF")
			}
			t.Logf("%s: %d -> %d bytes", name, len(data), len(result.Data))
		})
	}
}

// --- AVIF Fast ---

func TestAvifEncodeFast(t *testing.T) {
	files := append(jpegFiles, pngFiles...)
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			data := loadTestData(t, name)
			result, err := Avif.EncodeFast(data, 80, 0)
			if err != nil {
				t.Fatalf("Avif.EncodeFast(%s) failed: %v", name, err)
			}
			if !isAVIF(result.Data) {
				t.Fatal("output is not AVIF")
			}
			t.Logf("%s: %d -> %d bytes", name, len(data), len(result.Data))
		})
	}
}

// --- Error cases ---

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
