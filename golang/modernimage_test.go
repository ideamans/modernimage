package modernimage

import (
	"fmt"
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

// --- JPEG Orientation ---

// injectExifOrientation creates a new JPEG by inserting an APP1 (Exif) marker
// with the given orientation value into an existing JPEG that has no EXIF.
func injectExifOrientation(t *testing.T, jpeg []byte, orientation int) []byte {
	t.Helper()
	if len(jpeg) < 2 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		t.Fatal("not a JPEG")
	}

	// Build minimal EXIF APP1 segment:
	// FF E1 [length:2] "Exif\0\0" [TIFF header] [IFD0 with orientation tag]
	// TIFF header: "MM" (big-endian) + 0x002A + offset to IFD0 (8)
	// IFD0: count=1, tag=0x0112 type=SHORT count=1 value=orientation, next_ifd=0
	exif := []byte{
		0xFF, 0xE1, // APP1 marker
		0x00, 0x22, // length = 34 (includes these 2 bytes)
		'E', 'x', 'i', 'f', 0x00, 0x00, // "Exif\0\0"
		'M', 'M', // big-endian
		0x00, 0x2A, // TIFF magic
		0x00, 0x00, 0x00, 0x08, // offset to IFD0
		0x00, 0x01, // 1 IFD entry
		0x01, 0x12, // tag: Orientation (0x0112)
		0x00, 0x03, // type: SHORT
		0x00, 0x00, 0x00, 0x01, // count: 1
		0x00, byte(orientation), 0x00, 0x00, // value
		0x00, 0x00, 0x00, 0x00, // next IFD offset: 0 (no more IFDs)
	}

	// Insert after SOI (FF D8)
	result := make([]byte, 0, len(jpeg)+len(exif))
	result = append(result, jpeg[:2]...) // SOI
	result = append(result, exif...)
	result = append(result, jpeg[2:]...) // rest of JPEG
	return result
}

func TestJpegOrientation(t *testing.T) {
	jpeg := loadTestData(t, "small-128x128.jpg")

	t.Run("no_exif", func(t *testing.T) {
		ori := JpegOrientation(jpeg)
		if ori != 0 {
			t.Fatalf("expected 0 for JPEG without EXIF, got %d", ori)
		}
	})

	for _, tc := range []struct {
		name        string
		orientation int
	}{
		{"orientation_1", 1},
		{"orientation_2", 2},
		{"orientation_3", 3},
		{"orientation_6", 6},
		{"orientation_8", 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := injectExifOrientation(t, jpeg, tc.orientation)
			ori := JpegOrientation(data)
			if ori != tc.orientation {
				t.Fatalf("expected %d, got %d", tc.orientation, ori)
			}
		})
	}

	t.Run("not_jpeg", func(t *testing.T) {
		png := loadTestData(t, "small-128x128.png")
		ori := JpegOrientation(png)
		if ori != 0 {
			t.Fatalf("expected 0 for PNG, got %d", ori)
		}
	})

	t.Run("empty", func(t *testing.T) {
		ori := JpegOrientation(nil)
		if ori != 0 {
			t.Fatalf("expected 0 for empty, got %d", ori)
		}
	})
}

func TestNormalizeJpegOrientation(t *testing.T) {
	jpeg := loadTestData(t, "small-128x128.jpg")

	t.Run("no_rotation_needed", func(t *testing.T) {
		result, err := NormalizeJpegOrientation(jpeg)
		if err != nil {
			t.Fatal(err)
		}
		// Should return same data when no EXIF orientation
		if &result[0] != &jpeg[0] {
			t.Fatal("expected same slice returned when no orientation")
		}
	})

	t.Run("orientation_1_noop", func(t *testing.T) {
		data := injectExifOrientation(t, jpeg, 1)
		result, err := NormalizeJpegOrientation(data)
		if err != nil {
			t.Fatal(err)
		}
		if &result[0] != &data[0] {
			t.Fatal("expected same slice returned for orientation=1")
		}
	})

	for _, ori := range []int{2, 3, 4, 5, 6, 7, 8} {
		t.Run(fmt.Sprintf("orientation_%d", ori), func(t *testing.T) {
			data := injectExifOrientation(t, jpeg, ori)
			result, err := NormalizeJpegOrientation(data)
			if err != nil {
				t.Fatalf("orientation %d: %v", ori, err)
			}
			// Result should be valid JPEG
			if len(result) < 2 || result[0] != 0xFF || result[1] != 0xD8 {
				t.Fatal("result is not valid JPEG")
			}
			// Result should have no EXIF orientation (or orientation=1)
			newOri := JpegOrientation(result)
			if newOri > 1 {
				t.Fatalf("result still has orientation %d", newOri)
			}
			t.Logf("orientation %d: %d -> %d bytes", ori, len(data), len(result))
		})
	}
}

func TestNormalizeJpegOrientationThenWebP(t *testing.T) {
	jpeg := loadTestData(t, "small-128x128.jpg")
	data := injectExifOrientation(t, jpeg, 6) // 90 CW rotation

	normalized, err := NormalizeJpegOrientation(data)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Webp.EncodeLossy(normalized, 80, false)
	if err != nil {
		t.Fatalf("WebP encode after normalize: %v", err)
	}
	if !isWebP(result.Data) {
		t.Fatal("output is not WebP")
	}
	t.Logf("orient6 JPEG -> normalize -> WebP: %d -> %d -> %d bytes",
		len(data), len(normalized), len(result.Data))
}

func TestNormalizeJpegOrientationThenAVIF(t *testing.T) {
	jpeg := loadTestData(t, "small-128x128.jpg")
	data := injectExifOrientation(t, jpeg, 3) // 180 rotation

	normalized, err := NormalizeJpegOrientation(data)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Avif.EncodeFast(normalized, 80, 0)
	if err != nil {
		t.Fatalf("AVIF encode after normalize: %v", err)
	}
	if !isAVIF(result.Data) {
		t.Fatal("output is not AVIF")
	}
	t.Logf("orient3 JPEG -> normalize -> AVIF: %d -> %d -> %d bytes",
		len(data), len(normalized), len(result.Data))
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
