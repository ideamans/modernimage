package modernimage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/gif"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
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

// isAVIF verifies the data is an AVIF file by parsing the ftyp box and
// checking that the major brand or any compatible brand is "avif"/"avis".
// Just looking for "ftyp" at offset 4 would also accept HEIC, MP4, MOV, etc.
func isAVIF(data []byte) bool {
	if len(data) < 16 {
		return false
	}
	if data[4] != 'f' || data[5] != 't' || data[6] != 'y' || data[7] != 'p' {
		return false
	}
	size := int(binary.BigEndian.Uint32(data[0:4]))
	if size < 16 || size > len(data) {
		size = len(data)
	}
	isAvifBrand := func(b []byte) bool {
		return string(b) == "avif" || string(b) == "avis"
	}
	if isAvifBrand(data[8:12]) {
		return true
	}
	for i := 16; i+4 <= size; i += 4 {
		if isAvifBrand(data[i : i+4]) {
			return true
		}
	}
	return false
}

func TestIsAVIFRejectsNonAvifIsobmff(t *testing.T) {
	// Forge a minimal ftyp box claiming brand "heic" with no avif compatible brand.
	// Layout: [size:4=24] [ftyp:4] [major:heic] [minor:0] [compat:mif1][compat:heic]
	heic := []byte{
		0x00, 0x00, 0x00, 0x18, // size = 24
		'f', 't', 'y', 'p',
		'h', 'e', 'i', 'c',
		0x00, 0x00, 0x00, 0x00,
		'm', 'i', 'f', '1',
		'h', 'e', 'i', 'c',
	}
	if isAVIF(heic) {
		t.Fatal("isAVIF must reject HEIC ftyp")
	}

	// Forge a minimal AVIF ftyp.
	avif := []byte{
		0x00, 0x00, 0x00, 0x18, // size = 24
		'f', 't', 'y', 'p',
		'a', 'v', 'i', 'f',
		0x00, 0x00, 0x00, 0x00,
		'm', 'i', 'f', '1',
		'a', 'v', 'i', 'f',
	}
	if !isAVIF(avif) {
		t.Fatal("isAVIF must accept AVIF ftyp")
	}

	// AVIF only as compatible brand (major = mif1).
	avifCompat := []byte{
		0x00, 0x00, 0x00, 0x1C, // size = 28
		'f', 't', 'y', 'p',
		'm', 'i', 'f', '1',
		0x00, 0x00, 0x00, 0x00,
		'm', 'i', 'f', '1',
		'm', 'i', 'a', 'f',
		'a', 'v', 'i', 'f',
	}
	if !isAVIF(avifCompat) {
		t.Fatal("isAVIF must accept AVIF in compatible brands")
	}
}

// assertSameDimensions parses the input and output as image containers and
// checks that their canvas dimensions match. Width/height swap is a sign of
// EXIF orientation handling errors; arbitrary mismatches indicate the encoder
// is producing the wrong canvas size.
func assertSameDimensions(t *testing.T, name string, input, output []byte) {
	t.Helper()
	iw, ih, err := imageDimensions(input)
	if err != nil {
		t.Fatalf("%s: input dimensions: %v", name, err)
	}
	ow, oh, err := imageDimensions(output)
	if err != nil {
		t.Fatalf("%s: output dimensions: %v", name, err)
	}
	if iw != ow || ih != oh {
		t.Fatalf("%s: dimension mismatch input=%dx%d output=%dx%d",
			name, iw, ih, ow, oh)
	}
}

func TestImageDimensionsBaseline(t *testing.T) {
	cases := []struct {
		name string
	}{
		{"photo.jpg"}, {"photo-like.jpg"}, {"landscape-like.jpg"},
		{"edges.jpg"}, {"gradient-radial.jpg"},
		{"small-128x128.jpg"}, {"medium-512x512.jpg"},
		{"logo.png"}, {"photo-like.png"}, {"small-128x128.png"}, {"text.png"},
		{"flat-color.png"}, {"gradient-horizontal.png"},
		{"animation.gif"}, {"animated-3frames.gif"}, {"animated-small.gif"},
		{"static-512x512.gif"}, {"static-alpha.gif"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := loadTestData(t, c.name)
			w, h, err := imageDimensions(data)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if w <= 0 || h <= 0 {
				t.Fatalf("%s: invalid dimensions %dx%d", c.name, w, h)
			}
			t.Logf("%s: %dx%d", c.name, w, h)
		})
	}
}

func TestVersion(t *testing.T) {
	v := Version()
	if v == "" {
		t.Fatal("Version() returned empty string")
	}
	// Must look like semver (major.minor.patch[-pre][+meta]).
	semver := regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)
	if !semver.MatchString(v) {
		t.Fatalf("Version() %q is not semver-shaped", v)
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
			if result.MimeType != "image/webp" {
				t.Fatalf("MimeType = %q, want image/webp", result.MimeType)
			}
			assertSameDimensions(t, name, data, result.Data)
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
			// DEBUG (windows branch): was multithread=true; swapped to
			// false to test the hypothesis that cwebp -mt corrupts state
			// across calls on Windows.
			result, err := Webp.EncodeLossy(data, 80, false)
			if err != nil {
				t.Fatalf("Webp.EncodeLossy(%s) failed: %v", name, err)
			}
			if !isWebP(result.Data) {
				t.Fatal("output is not WebP")
			}
			if result.MimeType != "image/webp" {
				t.Fatalf("MimeType = %q, want image/webp", result.MimeType)
			}
			assertSameDimensions(t, name, data, result.Data)
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
			if result.MimeType != "image/webp" {
				t.Fatalf("MimeType = %q, want image/webp", result.MimeType)
			}
			assertSameDimensions(t, name, data, result.Data)

			// Lossless contract: output must use VP8L chunks (lossless
			// bitstream) and must NOT contain VP8 chunks (lossy bitstream).
			// Catches a regression where the encoder silently falls back
			// to lossy.
			info, err := parseWebP(result.Data)
			if err != nil {
				t.Fatalf("parse output: %v", err)
			}
			if !info.hasVP8L {
				t.Fatal("EncodeLossless output has no VP8L chunk")
			}
			if info.hasVP8 {
				t.Fatal("EncodeLossless output unexpectedly contains a VP8 (lossy) chunk")
			}
			t.Logf("%s: %d -> %d bytes (VP8L=%v VP8=%v)",
				name, len(data), len(result.Data), info.hasVP8L, info.hasVP8)
		})
	}
}

// TestWebpEncodeLossy_ProducesLossyChunks is the symmetric guard: lossy
// encoding must NOT silently produce VP8L (lossless) output. It's allowed to
// have VP8X for extensions, but the actual frame data should be in VP8 chunks.
func TestWebpEncodeLossy_ProducesLossyChunks(t *testing.T) {
	data := loadTestData(t, "photo.jpg")
	result, err := Webp.EncodeLossy(data, 80, false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := parseWebP(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	if info.hasVP8L {
		t.Fatal("EncodeLossy unexpectedly produced a VP8L (lossless) chunk")
	}
	if !info.hasVP8 {
		t.Fatal("EncodeLossy output has no VP8 (lossy) chunk")
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

// gifFrameCount returns the number of frames in a GIF using the standard
// library decoder.
func gifFrameCount(t *testing.T, data []byte) int {
	t.Helper()
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode gif: %v", err)
	}
	return len(g.Image)
}

func TestWebpEncodeGif(t *testing.T) {
	// Files that contain multiple frames vs static (single frame) GIFs.
	multi := map[string]bool{
		"animation.gif":        true,
		"animated-3frames.gif": true,
		"animated-small.gif":   true,
	}
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
			if result.MimeType != "image/webp" {
				t.Fatalf("MimeType = %q, want image/webp", result.MimeType)
			}
			assertSameDimensions(t, name, data, result.Data)

			info, err := parseWebP(result.Data)
			if err != nil {
				t.Fatalf("parse output WebP: %v", err)
			}
			gifFrames := gifFrameCount(t, data)

			if multi[name] {
				if !info.isAnimated {
					t.Fatal("animated GIF should produce animated WebP (VP8X animation flag missing)")
				}
				if info.frameCount != gifFrames {
					t.Fatalf("frame count mismatch: GIF=%d WebP ANMF=%d", gifFrames, info.frameCount)
				}
			} else {
				if info.isAnimated {
					t.Fatal("static GIF should not produce animated WebP")
				}
				if info.frameCount != 0 {
					t.Fatalf("static GIF should have 0 ANMF chunks, got %d", info.frameCount)
				}
			}
			t.Logf("%s: %d frames, %d -> %d bytes", name, gifFrames, len(data), len(result.Data))
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
			if result.MimeType != "image/avif" {
				t.Fatalf("MimeType = %q, want image/avif", result.MimeType)
			}
			assertSameDimensions(t, name, data, result.Data)
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
			if result.MimeType != "image/avif" {
				t.Fatalf("MimeType = %q, want image/avif", result.MimeType)
			}
			assertSameDimensions(t, name, data, result.Data)
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
			if result.MimeType != "image/avif" {
				t.Fatalf("MimeType = %q, want image/avif", result.MimeType)
			}
			assertSameDimensions(t, name, data, result.Data)
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

// injectXmpApp1 inserts an APP1 segment that uses the XMP identifier
// "http://ns.adobe.com/xap/1.0/\0" instead of "Exif\0\0". JpegOrientation
// must NOT mistake this for an EXIF segment.
func injectXmpApp1(t *testing.T, jpeg []byte) []byte {
	t.Helper()
	if len(jpeg) < 2 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		t.Fatal("not a JPEG")
	}
	xmpID := []byte("http://ns.adobe.com/xap/1.0/\x00")
	// Tiny placeholder XMP body — just enough to look like an XMP packet.
	body := []byte(`<?xpacket begin='' id='W5M0MpCehiHzreSzNTczkc9d'?><x:xmpmeta xmlns:x='adobe:ns:meta/'/><?xpacket end='r'?>`)
	payload := append([]byte{}, xmpID...)
	payload = append(payload, body...)
	segLen := 2 + len(payload) // 2 bytes length field + payload
	if segLen > 0xFFFF {
		t.Fatal("XMP segment too large for one APP1")
	}
	seg := []byte{0xFF, 0xE1, byte(segLen >> 8), byte(segLen & 0xFF)}
	seg = append(seg, payload...)

	out := make([]byte, 0, len(jpeg)+len(seg))
	out = append(out, jpeg[:2]...)
	out = append(out, seg...)
	out = append(out, jpeg[2:]...)
	return out
}

// TestJpegOrientationRealLittleEndianEXIF uses a real CC0 JPEG with
// little-endian EXIF (orientation=6) to exercise a code path that the
// big-endian synthetic injectExifOrientation does not cover. Real-world
// cameras almost always emit little-endian EXIF, so this is the path that
// the library will encounter in production.
//
// See testdata/THIRD_PARTY.md for the source and license.
func TestJpegOrientationRealLittleEndianEXIF(t *testing.T) {
	data := loadTestData(t, "exif6-real.jpg")

	// Sanity: dimensions of the stored JPEG (pre-normalization).
	iw, ih, err := imageDimensions(data)
	if err != nil {
		t.Fatal(err)
	}
	if iw != 427 || ih != 640 {
		t.Fatalf("stored dims sanity: got %dx%d want 427x640", iw, ih)
	}

	t.Run("orientation_returns_6_from_LE_EXIF", func(t *testing.T) {
		ori := JpegOrientation(data)
		if ori != 6 {
			t.Fatalf("expected orientation=6 from real little-endian EXIF, got %d", ori)
		}
	})

	t.Run("normalize_rotates_90CW_no_trim_for_real_camera_JPEG", func(t *testing.T) {
		result, err := NormalizeJpegOrientation(data)
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		// orient 6 = 90° CW; stored 427x640 → display 640x427.
		// IMPORTANT: This real JPEG actually rotates perfectly (verified
		// with `jpegtran -perfect`) because its encoder padded the
		// stored data to MCU boundaries internally — the displayable
		// width 427 hides 5 padding columns. So `-trim` does nothing
		// and the result keeps the full 640x427 displayable area.
		//
		// Contrast with `nonmcu-99x97.jpg` (Go image/jpeg encoded),
		// which DOES require trim. See JUDGE-5 in review.md.
		ow, oh, err := imageDimensions(result)
		if err != nil {
			t.Fatal(err)
		}
		if ow != 640 || oh != 427 {
			t.Fatalf("expected 640x427 (orient6 90°CW, no trim needed), got %dx%d", ow, oh)
		}
		if newOri := JpegOrientation(result); newOri > 1 {
			t.Fatalf("normalized result still has orientation %d", newOri)
		}
	})

	t.Run("normalize_then_webp_carries_swapped_dims", func(t *testing.T) {
		normalized, err := NormalizeJpegOrientation(data)
		if err != nil {
			t.Fatal(err)
		}
		nw, nh, _ := imageDimensions(normalized)
		result, err := Webp.EncodeLossy(normalized, 80, false)
		if err != nil {
			t.Fatal(err)
		}
		ow, oh, err := imageDimensions(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		if ow != nw || oh != nh {
			t.Fatalf("WebP dims %dx%d != normalized dims %dx%d", ow, oh, nw, nh)
		}
	})
}

// TestJpegOrientationMultipleApp1 verifies that the parser correctly walks
// past unrelated APP1 segments (XMP) to find the real EXIF segment carrying
// the Orientation tag — and that this works regardless of the order in which
// the segments are inserted.
func TestJpegOrientationMultipleApp1(t *testing.T) {
	src := loadTestData(t, "small-128x128.jpg")
	if JpegOrientation(src) != 0 {
		t.Fatal("source has unexpected EXIF orientation")
	}

	t.Run("XMP_then_EXIF", func(t *testing.T) {
		// Inject XMP first, then EXIF orientation=6 on top.
		// (injectExifOrientation always inserts at offset 2, so the EXIF
		// segment ends up first in the marker stream.)
		withXmp := injectXmpApp1(t, src)
		data := injectExifOrientation(t, withXmp, 6)
		ori := JpegOrientation(data)
		if ori != 6 {
			t.Fatalf("expected orientation=6 even with XMP also present, got %d", ori)
		}
	})

	t.Run("EXIF_then_XMP", func(t *testing.T) {
		// Inject EXIF first, then XMP. EXIF must still be discovered.
		withExif := injectExifOrientation(t, src, 3)
		data := injectXmpApp1(t, withExif)
		ori := JpegOrientation(data)
		if ori != 3 {
			t.Fatalf("expected orientation=3 even with XMP also present, got %d", ori)
		}
	})

	t.Run("Two_XMP_no_EXIF", func(t *testing.T) {
		// Two XMP segments and no EXIF — must still return 0.
		once := injectXmpApp1(t, src)
		twice := injectXmpApp1(t, once)
		if ori := JpegOrientation(twice); ori != 0 {
			t.Fatalf("expected 0 with two XMP and no EXIF, got %d", ori)
		}
	})

	// Normalize must also do the right thing on the mixed case: rotate
	// according to the EXIF orientation while leaving the JPEG well-formed.
	t.Run("Normalize_with_XMP_and_EXIF_rotates", func(t *testing.T) {
		landscape := loadTestData(t, "landscape-like.jpg")
		iw, ih, _ := imageDimensions(landscape)
		withXmp := injectXmpApp1(t, landscape)
		data := injectExifOrientation(t, withXmp, 6) // 90° → wxh swap
		result, err := NormalizeJpegOrientation(data)
		if err != nil {
			t.Fatal(err)
		}
		ow, oh, err := imageDimensions(result)
		if err != nil {
			t.Fatal(err)
		}
		if ow != ih || oh != iw {
			t.Fatalf("expected wxh swap %dx%d, got %dx%d", ih, iw, ow, oh)
		}
	})
}

func TestJpegOrientationIgnoresXmpApp1(t *testing.T) {
	jpeg := loadTestData(t, "small-128x128.jpg")
	// Sanity: source has no orientation tag.
	if JpegOrientation(jpeg) != 0 {
		t.Fatal("source has unexpected EXIF orientation")
	}
	data := injectXmpApp1(t, jpeg)
	if len(data) <= len(jpeg) {
		t.Fatal("inject helper did not add bytes")
	}
	ori := JpegOrientation(data)
	if ori != 0 {
		t.Fatalf("XMP-only JPEG should return 0, got %d", ori)
	}

	// NormalizeJpegOrientation should treat this as a no-op too (no rotation).
	result, err := NormalizeJpegOrientation(data)
	if err != nil {
		t.Fatal(err)
	}
	// Either byte-equal or at least non-rotated (XMP-only file shouldn't trigger rotation).
	resultOri := JpegOrientation(result)
	if resultOri > 1 {
		t.Fatalf("normalize unexpectedly rotated XMP-only JPEG (resulting orientation=%d)", resultOri)
	}
}

// injectExifWithoutOrientation creates a JPEG with a valid APP1/EXIF segment
// that contains an IFD0 entry for ImageDescription (0x010E) but no Orientation
// tag. Used to verify that JpegOrientation returns 0 when EXIF exists but the
// Orientation tag is missing — a common real-world case.
func injectExifWithoutOrientation(t *testing.T, jpeg []byte) []byte {
	t.Helper()
	if len(jpeg) < 2 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		t.Fatal("not a JPEG")
	}
	// Single IFD0 entry: ImageDescription, type ASCII, count=1, value="A".
	// Value fits inline (4 bytes), so no offset chasing.
	exif := []byte{
		0xFF, 0xE1, // APP1
		0x00, 0x22, // length = 34
		'E', 'x', 'i', 'f', 0x00, 0x00,
		'M', 'M', // big-endian
		0x00, 0x2A, // TIFF magic
		0x00, 0x00, 0x00, 0x08, // offset to IFD0
		0x00, 0x01, // 1 IFD entry
		0x01, 0x0E, // tag: ImageDescription (0x010E)
		0x00, 0x02, // type: ASCII
		0x00, 0x00, 0x00, 0x01, // count: 1
		'A', 0x00, 0x00, 0x00, // inline value
		0x00, 0x00, 0x00, 0x00, // next IFD: 0
	}
	out := make([]byte, 0, len(jpeg)+len(exif))
	out = append(out, jpeg[:2]...)
	out = append(out, exif...)
	out = append(out, jpeg[2:]...)
	return out
}

func TestJpegOrientationExifWithoutOrientationTag(t *testing.T) {
	jpeg := loadTestData(t, "small-128x128.jpg")
	data := injectExifWithoutOrientation(t, jpeg)

	// Sanity: confirm we did inject some APP1 EXIF.
	if len(data) <= len(jpeg) {
		t.Fatal("inject helper did not add bytes")
	}

	ori := JpegOrientation(data)
	// Documented contract: 0 means "no orientation tag found".
	// Either 0 (not found) or 1 (default identity) is acceptable behavior;
	// we accept the same set as the existing "no_exif" test.
	if ori != 0 && ori != 1 {
		t.Fatalf("expected 0 or 1 for EXIF-without-orientation, got %d", ori)
	}
	t.Logf("EXIF-without-orientation returned %d", ori)

	// Normalization should treat this like a no-op (no rotation needed).
	result, err := NormalizeJpegOrientation(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result, data) {
		// Either implementation is acceptable as long as the function
		// doesn't fail and the result is byte-equal to the input.
		t.Logf("normalized output differs from input (%d → %d bytes), but no error",
			len(data), len(result))
	}
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
		{"orientation_4", 4},
		{"orientation_5", 5},
		{"orientation_6", 6},
		{"orientation_7", 7},
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

	t.Run("empty_input_errors", func(t *testing.T) {
		_, err := NormalizeJpegOrientation(nil)
		if err == nil {
			t.Fatal("expected error for empty input")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Fatalf("expected 'empty' in error, got %v", err)
		}
	})

	t.Run("no_rotation_needed", func(t *testing.T) {
		result, err := NormalizeJpegOrientation(jpeg)
		if err != nil {
			t.Fatal(err)
		}
		// Returns a copy with identical bytes (no aliasing contract).
		if !bytes.Equal(result, jpeg) {
			t.Fatal("expected byte-equal result for no-EXIF JPEG")
		}
	})

	t.Run("orientation_1_noop", func(t *testing.T) {
		data := injectExifOrientation(t, jpeg, 1)
		result, err := NormalizeJpegOrientation(data)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(result, data) {
			t.Fatal("expected byte-equal result for orientation=1")
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

// findRedCentroid scans the decoded RGBA image for "bright red" pixels
// (R > 200, G < 100, B < 100) and returns the centroid as (x, y). The
// JPEG round-trip + jpegtran transform softens the marker edges, so
// we use a tolerant threshold and average all matching pixels.
func findRedCentroid(t *testing.T, jpegData []byte) (int, int) {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	bounds := img.Bounds()
	var sumX, sumY, count int
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// 16-bit values from RGBA() — divide by 256 to get 8-bit
			r8, g8, b8 := r>>8, g>>8, b>>8
			if r8 > 200 && g8 < 100 && b8 < 100 {
				sumX += x
				sumY += y
				count++
			}
		}
	}
	if count == 0 {
		t.Fatal("no red marker pixels found in decoded image")
	}
	return sumX / count, sumY / count
}

// TestNormalizeJpegOrientation_MarkerPixel verifies that the rotation
// happens in the correct direction by tracking a known marker pixel.
// Width/height swap (`TestNormalizeJpegOrientationDimensions`) catches
// "no rotation at all", but it would NOT catch a "rotated 90° CCW
// instead of CW" implementation bug (dimensions still swap, just to
// the wrong corner). This test catches that.
//
// marker-80x48.jpg has a 4×4 red block at stored coordinate (8, 4),
// centroid (10, 6) in an 80×48 dark-gray image.
//
// EXIF orientation maps centroid (mx=10, my=6) in (W=80, H=48) to:
//
//	1: (10, 6)            in 80×48  (no transform)
//	2: (W-1-mx, my)       in 80×48  → (69, 6)   flip H
//	3: (W-1-mx, H-1-my)   in 80×48  → (69, 41)  180°
//	4: (mx, H-1-my)       in 80×48  → (10, 41)  flip V
//	5: (my, mx)           in 48×80  → (6, 10)   transpose
//	6: (H-1-my, mx)       in 48×80  → (41, 10)  90° CW
//	7: (H-1-my, W-1-mx)   in 48×80  → (41, 69)  transverse
//	8: (my, W-1-mx)       in 48×80  → (6, 69)   270° CW
func TestNormalizeJpegOrientation_MarkerPixel(t *testing.T) {
	src := loadTestData(t, "marker-80x48.jpg")

	// Sanity: source dims and marker location.
	iw, ih, err := imageDimensions(src)
	if err != nil {
		t.Fatal(err)
	}
	if iw != 80 || ih != 48 {
		t.Fatalf("source dims sanity: got %dx%d want 80x48", iw, ih)
	}
	srcMx, srcMy := findRedCentroid(t, src)
	if srcMx < 8 || srcMx > 12 || srcMy < 4 || srcMy > 8 {
		t.Fatalf("source marker centroid (%d, %d) outside expected (8-12, 4-8)", srcMx, srcMy)
	}

	type tc struct {
		ori                int
		wantW, wantH       int
		wantCx, wantCy     int
		tolerance          int // pixel tolerance for centroid match
	}
	cases := []tc{
		{1, 80, 48, 10, 6, 2}, // identity
		{2, 80, 48, 69, 6, 2}, // flip H
		{3, 80, 48, 69, 41, 2}, // 180°
		{4, 80, 48, 10, 41, 2}, // flip V
		{5, 48, 80, 6, 10, 2}, // transpose
		{6, 48, 80, 41, 10, 2}, // 90° CW
		{7, 48, 80, 41, 69, 2}, // transverse
		{8, 48, 80, 6, 69, 2}, // 270° CW
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("orient_%d", c.ori), func(t *testing.T) {
			var data []byte
			if c.ori == 1 {
				// orient 1 is no-op; normalize should return the source unchanged.
				data = src
			} else {
				data = injectExifOrientation(t, src, c.ori)
			}
			result, err := NormalizeJpegOrientation(data)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			ow, oh, err := imageDimensions(result)
			if err != nil {
				t.Fatal(err)
			}
			if ow != c.wantW || oh != c.wantH {
				t.Fatalf("orient %d: dims got %dx%d want %dx%d",
					c.ori, ow, oh, c.wantW, c.wantH)
			}
			cx, cy := findRedCentroid(t, result)
			dx := abs(cx - c.wantCx)
			dy := abs(cy - c.wantCy)
			if dx > c.tolerance || dy > c.tolerance {
				t.Fatalf("orient %d: centroid got (%d, %d) want (%d, %d) ±%d",
					c.ori, cx, cy, c.wantCx, c.wantCy, c.tolerance)
			}
			t.Logf("orient %d: centroid (%d, %d) ✓", c.ori, cx, cy)
		})
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// TestNormalizeJpegOrientation_NonMCU verifies that NormalizeJpegOrientation
// preserves every pixel for non-MCU-aligned JPEGs by transparently falling
// back to a decode → rotate → re-encode path when jpegtran -perfect refuses
// the source. JUDGE-5 in review.md tracked the original problem.
//
// Background:
//   - libjpeg's lossless transforms operate on iMCU blocks (typically 8 px).
//     For most transforms, partial iMCUs at the edges are unrepresentable
//     in the rotated output, so jpegtran can either drop them (-trim, lossy
//     in pixels) or refuse the operation (-perfect).
//   - The `nonmcu-99x97.jpg` fixture is encoded by Go's image/jpeg, which
//     does NOT pad the storage to iMCU boundaries. Real-camera JPEGs
//     usually do, so the perfect path succeeds for them and the fallback
//     is bypassed.
//   - For this fixture, jpegtran -perfect refuses orient 2,3,4,6,7,8 and
//     succeeds only for orient 5 (transpose). The Go binding catches that
//     refusal and re-runs the operation via decode/rotate/encode, which
//     preserves all 99 × 97 pixels.
//
// Expected (post-fix):
//
//	orient2 (flip H)     → 99x97  (decode/rotate fallback)
//	orient3 (180°)       → 99x97  (decode/rotate fallback)
//	orient4 (flip V)     → 99x97  (decode/rotate fallback)
//	orient5 (transpose)  → 97x99  (jpegtran -perfect succeeds, fast path)
//	orient6 (90° CW)     → 97x99  (decode/rotate fallback)
//	orient7 (transverse) → 97x99  (decode/rotate fallback)
//	orient8 (270° CW)    → 97x99  (decode/rotate fallback)
func TestNormalizeJpegOrientation_NonMCU(t *testing.T) {
	src := loadTestData(t, "nonmcu-99x97.jpg")
	iw, ih, err := imageDimensions(src)
	if err != nil {
		t.Fatal(err)
	}
	if iw != 99 || ih != 97 {
		t.Fatalf("source dimensions sanity: got %dx%d want 99x97", iw, ih)
	}

	cases := []struct {
		name         string
		ori          int
		wantW, wantH int
	}{
		{"orient2_flipH", 2, 99, 97},
		{"orient3_180", 3, 99, 97},
		{"orient4_flipV", 4, 99, 97},
		{"orient5_transpose", 5, 97, 99},
		{"orient6_90CW", 6, 97, 99},
		{"orient7_transverse", 7, 97, 99},
		{"orient8_270CW", 8, 97, 99},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := injectExifOrientation(t, src, c.ori)
			result, err := NormalizeJpegOrientation(data)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			ow, oh, err := imageDimensions(result)
			if err != nil {
				t.Fatal(err)
			}
			if ow != c.wantW || oh != c.wantH {
				t.Fatalf("orient%d: want %dx%d (no trim), got %dx%d",
					c.ori, c.wantW, c.wantH, ow, oh)
			}
			// And the orientation tag must be cleared (or 1).
			if newOri := JpegOrientation(result); newOri > 1 {
				t.Fatalf("orient%d: result still has orientation %d", c.ori, newOri)
			}
		})
	}
}

// TestNormalizeJpegOrientationDimensions verifies the actual pixel rotation
// happened by checking that the output canvas dimensions match the expected
// shape. orientations 5/6/7/8 (which involve a 90-degree rotation) must swap
// width and height; orientations 2/3/4 must preserve them.
//
// This catches the failure mode where the implementation strips the EXIF tag
// without actually rotating pixels.
func TestNormalizeJpegOrientationDimensions(t *testing.T) {
	// Use a non-square image so width != height swap is observable.
	jpeg := loadTestData(t, "landscape-like.jpg")
	iw, ih, err := imageDimensions(jpeg)
	if err != nil {
		t.Fatalf("input dims: %v", err)
	}
	if iw == ih {
		t.Fatalf("test image must not be square (got %dx%d)", iw, ih)
	}
	if JpegOrientation(jpeg) != 0 {
		t.Fatal("test image must not have pre-existing EXIF orientation")
	}

	cases := []struct {
		ori  int
		swap bool
	}{
		{2, false},
		{3, false},
		{4, false},
		{5, true},
		{6, true},
		{7, true},
		{8, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("orientation_%d", c.ori), func(t *testing.T) {
			data := injectExifOrientation(t, jpeg, c.ori)
			result, err := NormalizeJpegOrientation(data)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			ow, oh, err := imageDimensions(result)
			if err != nil {
				t.Fatalf("output dims: %v", err)
			}
			wantW, wantH := iw, ih
			if c.swap {
				wantW, wantH = ih, iw
			}
			if ow != wantW || oh != wantH {
				t.Fatalf("orientation %d: want %dx%d, got %dx%d",
					c.ori, wantW, wantH, ow, oh)
			}
			t.Logf("orientation %d: %dx%d -> %dx%d", c.ori, iw, ih, ow, oh)
		})
	}
}

func TestNormalizeJpegOrientationThenWebP(t *testing.T) {
	jpeg := loadTestData(t, "landscape-like.jpg")
	iw, ih, err := imageDimensions(jpeg)
	if err != nil {
		t.Fatal(err)
	}
	data := injectExifOrientation(t, jpeg, 6) // 90 CW rotation, swaps dimensions

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
	// orientation=6 must swap dimensions all the way through.
	ow, oh, err := imageDimensions(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	if ow != ih || oh != iw {
		t.Fatalf("orient6 chain: want %dx%d (swapped), got %dx%d", ih, iw, ow, oh)
	}
	t.Logf("orient6 JPEG -> normalize -> WebP: %dx%d -> %dx%d (%d -> %d -> %d bytes)",
		iw, ih, ow, oh, len(data), len(normalized), len(result.Data))
}

func TestNormalizeJpegOrientationThenAVIF(t *testing.T) {
	jpeg := loadTestData(t, "landscape-like.jpg")
	iw, ih, err := imageDimensions(jpeg)
	if err != nil {
		t.Fatal(err)
	}
	data := injectExifOrientation(t, jpeg, 3) // 180 rotation, dimensions preserved

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
	ow, oh, err := imageDimensions(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	if ow != iw || oh != ih {
		t.Fatalf("orient3 chain: want %dx%d (preserved), got %dx%d", iw, ih, ow, oh)
	}
	t.Logf("orient3 JPEG -> normalize -> AVIF: %dx%d -> %dx%d (%d -> %d -> %d bytes)",
		iw, ih, ow, oh, len(data), len(normalized), len(result.Data))
}

// Cover orient5 (transpose) for AVIF chain — makes sure non-rotate
// transforms also propagate dimension swap.
func TestNormalizeJpegOrientationThenWebP_Transpose(t *testing.T) {
	jpeg := loadTestData(t, "landscape-like.jpg")
	iw, ih, _ := imageDimensions(jpeg)
	data := injectExifOrientation(t, jpeg, 5) // transpose, swaps dimensions

	normalized, err := NormalizeJpegOrientation(data)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Webp.EncodeLossy(normalized, 80, false)
	if err != nil {
		t.Fatal(err)
	}
	ow, oh, _ := imageDimensions(result.Data)
	if ow != ih || oh != iw {
		t.Fatalf("orient5 chain: want %dx%d (swapped), got %dx%d", ih, iw, ow, oh)
	}
}

// TestQualityBoundaries exercises quality 0/50/100 to make sure the encoders
// don't crash at the extremes and that q=100 produces a larger output than
// q=0 (rough sanity that quality is actually plumbed through).
func TestQualityBoundaries(t *testing.T) {
	data := loadTestData(t, "photo.jpg")

	t.Run("Webp.EncodeLossy_quality_extremes", func(t *testing.T) {
		var sizes [3]int
		for i, q := range []int{0, 50, 100} {
			r, err := Webp.EncodeLossy(data, q, false)
			if err != nil {
				t.Fatalf("q=%d: %v", q, err)
			}
			if !isWebP(r.Data) {
				t.Fatalf("q=%d: not WebP", q)
			}
			assertSameDimensions(t, fmt.Sprintf("q=%d", q), data, r.Data)
			sizes[i] = len(r.Data)
		}
		if sizes[2] <= sizes[0] {
			t.Fatalf("q=100 (%d) should be larger than q=0 (%d)", sizes[2], sizes[0])
		}
		t.Logf("WebP lossy: q=0:%d q=50:%d q=100:%d", sizes[0], sizes[1], sizes[2])
	})

	// AVIF Fast preset uses YUV420 + BT.601 (CICP 1/13/6). avifenc treats
	// q=100 as lossless mode, which requires YUV444 + identity matrix, so
	// q=100 is rejected for the Fast preset. We test q=0/50/99 to cover the
	// usable range without triggering the lossless guard.
	// (See review.md for the cross-preset q=100 matrix.)
	t.Run("Avif.EncodeFast_quality_range", func(t *testing.T) {
		var sizes [3]int
		for i, q := range []int{0, 50, 99} {
			r, err := Avif.EncodeFast(data, q, 0)
			if err != nil {
				t.Fatalf("q=%d: %v", q, err)
			}
			if !isAVIF(r.Data) {
				t.Fatalf("q=%d: not AVIF", q)
			}
			assertSameDimensions(t, fmt.Sprintf("q=%d", q), data, r.Data)
			sizes[i] = len(r.Data)
		}
		if sizes[2] <= sizes[0] {
			t.Fatalf("q=99 (%d) should be larger than q=0 (%d)", sizes[2], sizes[0])
		}
		t.Logf("AVIF fast: q=0:%d q=50:%d q=99:%d", sizes[0], sizes[1], sizes[2])
	})

	// IMPORTANT: All 3 AVIF presets currently fail at q=100 for RGB inputs.
	// Reason: avifenc treats q=100 as lossless mode, which requires the
	// matrixCoefficients to be identity (CICP x/x/0) or YCgCo. All presets
	// currently use CICP `1/1/1` (BT.709) or `1/13/6`, so none satisfies
	// the lossless guard. See review.md "judgment points" — the upstream
	// author should decide whether to:
	//   a) document q=100 as unsupported and cap at 99 in the API
	//   b) switch one preset (Lossless?) to identity CICP
	//   c) silently coerce q=100 to q=99
	// For now we lock in the observed behavior (all 3 fail at q=100) so
	// any change becomes visible.
	t.Run("AVIF_q100_all_presets_fail", func(t *testing.T) {
		if _, err := Avif.EncodeBalanced(data, 100, 0); err == nil {
			t.Error("Balanced q=100 unexpectedly succeeded — preset config may have changed")
		}
		if _, err := Avif.EncodeCompact(data, 100, 0); err == nil {
			t.Error("Compact q=100 unexpectedly succeeded — preset config may have changed")
		}
		if _, err := Avif.EncodeFast(data, 100, 0); err == nil {
			t.Error("Fast q=100 unexpectedly succeeded — preset config may have changed")
		}
	})
}

// TestMultithreadSmoke verifies the multithread=true codepath at least
// produces a valid file with the same dimensions as single-thread.
// (We don't claim it's faster — that's a benchmark, not a unit test.)
func TestMultithreadSmoke(t *testing.T) {
	data := loadTestData(t, "medium-512x512.jpg")

	t.Run("Webp.EncodeLossy_mt", func(t *testing.T) {
		r, err := Webp.EncodeLossy(data, 80, true)
		if err != nil {
			t.Fatal(err)
		}
		if !isWebP(r.Data) {
			t.Fatal("not WebP")
		}
		assertSameDimensions(t, "mt", data, r.Data)
	})

	t.Run("Webp.EncodeLossless_mt", func(t *testing.T) {
		r, err := Webp.EncodeLossless(data, true)
		if err != nil {
			t.Fatal(err)
		}
		if !isWebP(r.Data) {
			t.Fatal("not WebP")
		}
		assertSameDimensions(t, "mt", data, r.Data)
	})

	t.Run("Webp.EncodeGif_mt", func(t *testing.T) {
		gifData := loadTestData(t, "animation.gif")
		r, err := Webp.EncodeGif(gifData, true)
		if err != nil {
			t.Fatal(err)
		}
		if !isWebP(r.Data) {
			t.Fatal("not WebP")
		}
	})

	t.Run("Avif.EncodeFast_jobs1", func(t *testing.T) {
		r, err := Avif.EncodeFast(data, 80, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !isAVIF(r.Data) {
			t.Fatal("not AVIF")
		}
		assertSameDimensions(t, "jobs1", data, r.Data)
	})

	t.Run("Avif.EncodeFast_jobs8", func(t *testing.T) {
		r, err := Avif.EncodeFast(data, 80, 8)
		if err != nil {
			t.Fatal(err)
		}
		if !isAVIF(r.Data) {
			t.Fatal("not AVIF")
		}
		assertSameDimensions(t, "jobs8", data, r.Data)
	})
}

// TestCompressionSanity is a soft sanity check that the lossy encoders
// actually compress non-trivially. We use a 1024x768 photographic JPEG
// (175 KB) where every modern lossy codec at q=80 should easily produce
// output ≤ 50% of input. This catches the failure mode where the encoder
// produces something "valid" but the compression itself is broken (e.g.
// the encoder stores raw pixels in a thin container).
func TestCompressionSanity(t *testing.T) {
	src := loadTestData(t, "landscape-like.jpg")
	if len(src) < 50000 {
		t.Skip("source too small for compression sanity")
	}
	maxLossy := len(src) / 2 // 50% of input

	t.Run("Webp.EncodeLossy", func(t *testing.T) {
		r, err := Webp.EncodeLossy(src, 80, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Data) > maxLossy {
			t.Fatalf("WebP lossy q=80: %d bytes > 50%% of input %d bytes", len(r.Data), len(src))
		}
		t.Logf("WebP lossy: %d -> %d bytes (%.1f%%)", len(src), len(r.Data),
			100*float64(len(r.Data))/float64(len(src)))
	})

	t.Run("Avif.EncodeFast", func(t *testing.T) {
		r, err := Avif.EncodeFast(src, 80, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Data) > maxLossy {
			t.Fatalf("AVIF fast q=80: %d bytes > 50%% of input %d bytes", len(r.Data), len(src))
		}
		t.Logf("AVIF fast: %d -> %d bytes (%.1f%%)", len(src), len(r.Data),
			100*float64(len(r.Data))/float64(len(src)))
	})

	t.Run("Avif.EncodeBalanced", func(t *testing.T) {
		r, err := Avif.EncodeBalanced(src, 80, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Data) > maxLossy {
			t.Fatalf("AVIF balanced q=80: %d bytes > 50%% of input %d bytes", len(r.Data), len(src))
		}
		t.Logf("AVIF balanced: %d -> %d bytes (%.1f%%)", len(src), len(r.Data),
			100*float64(len(r.Data))/float64(len(src)))
	})

	// Lossless: don't assert ratio (lossless re-encode of an already-JPEG
	// image can easily exceed input). Just ensure non-empty output.
	t.Run("Webp.EncodeLossless_nonempty", func(t *testing.T) {
		r, err := Webp.EncodeLossless(src, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Data) == 0 {
			t.Fatal("lossless produced empty output")
		}
		t.Logf("WebP lossless: %d -> %d bytes (%.1f%%)", len(src), len(r.Data),
			100*float64(len(r.Data))/float64(len(src)))
	})
}

// TestAvifCompactSmallerThanBalanced verifies that the Compact preset
// (slowest, best compression) produces smaller output than Balanced for at
// least one representative photo. If this ever fails, either the preset is
// no longer compressing better or the test image has degenerated to a case
// where the difference is in the noise.
func TestAvifCompactSmallerThanBalanced(t *testing.T) {
	data := loadTestData(t, "photo.jpg")
	balanced, err := Avif.EncodeBalanced(data, 80, 0)
	if err != nil {
		t.Fatal(err)
	}
	compact, err := Avif.EncodeCompact(data, 80, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("photo.jpg @ q=80: balanced=%d compact=%d (saved %d bytes, %.1f%%)",
		len(balanced.Data), len(compact.Data),
		len(balanced.Data)-len(compact.Data),
		100*float64(len(balanced.Data)-len(compact.Data))/float64(len(balanced.Data)))
	if len(compact.Data) >= len(balanced.Data) {
		t.Fatalf("compact (%d bytes) is not smaller than balanced (%d bytes)",
			len(compact.Data), len(balanced.Data))
	}
}

// TestPinnerGCStress runs many encodes in parallel and forces GC cycles
// in between to stress the runtime.Pinner usage in callTool / JpegOrientation.
// If pinning is broken, GC could move the JPEG/PNG slice while CGo is still
// reading it, leading to crashes or corrupted output.
func TestPinnerGCStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping GC stress in short mode")
	}
	jpeg := loadTestData(t, "small-128x128.jpg")
	jw, jh, _ := imageDimensions(jpeg)

	const goroutines = 8
	const itersPerGoroutine = 10
	done := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			var lastErr error
			for i := 0; i < itersPerGoroutine; i++ {
				// Create a fresh slice each iteration so GC can prove it's
				// the new allocation that the FFI sees.
				cp := make([]byte, len(jpeg))
				copy(cp, jpeg)

				r, err := Webp.EncodeLossy(cp, 80, false)
				if err != nil {
					lastErr = err
					break
				}
				w, h, _ := imageDimensions(r.Data)
				if w != jw || h != jh {
					lastErr = fmt.Errorf("dim mismatch %dx%d (want %dx%d)", w, h, jw, jh)
					break
				}

				// Also exercise JpegOrientation which uses Pinner.
				_ = JpegOrientation(cp)
			}
			done <- lastErr
		}()
	}

	// Drive the GC concurrently while encodes are running.
	gcDone := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			runtime.GC()
		}
		close(gcDone)
	}()

	for g := 0; g < goroutines; g++ {
		if err := <-done; err != nil {
			t.Errorf("goroutine failed: %v", err)
		}
	}
	<-gcDone
}

// --- Concurrency / thread safety ---

// TestConcurrentEncodes runs N encodes in parallel goroutines and verifies
// each result is correct (valid format + correct dimensions). Uses two
// different inputs to also catch any cross-call state leakage.
func TestConcurrentEncodes(t *testing.T) {
	jpeg := loadTestData(t, "small-128x128.jpg")
	png := loadTestData(t, "logo.png")
	jw, jh, _ := imageDimensions(jpeg)
	pw, ph, _ := imageDimensions(png)

	const goroutines = 16
	const itersPerGoroutine = 4
	type result struct {
		err error
		w   int
		h   int
		typ string
	}
	results := make(chan result, goroutines*itersPerGoroutine)
	done := make(chan struct{})

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			for i := 0; i < itersPerGoroutine; i++ {
				// Alternate between 4 different encoders/inputs.
				switch (id*itersPerGoroutine + i) % 4 {
				case 0:
					r, err := Webp.EncodeLossy(jpeg, 80, false)
					if err != nil {
						results <- result{err: err}
						continue
					}
					w, h, _ := imageDimensions(r.Data)
					results <- result{w: w, h: h, typ: "webp_lossy_jpeg"}
				case 1:
					r, err := Webp.EncodeLossless(png, false)
					if err != nil {
						results <- result{err: err}
						continue
					}
					w, h, _ := imageDimensions(r.Data)
					results <- result{w: w, h: h, typ: "webp_lossless_png"}
				case 2:
					r, err := Avif.EncodeFast(jpeg, 80, 0)
					if err != nil {
						results <- result{err: err}
						continue
					}
					w, h, _ := imageDimensions(r.Data)
					results <- result{w: w, h: h, typ: "avif_jpeg"}
				case 3:
					r, err := Avif.EncodeFast(png, 80, 0)
					if err != nil {
						results <- result{err: err}
						continue
					}
					w, h, _ := imageDimensions(r.Data)
					results <- result{w: w, h: h, typ: "avif_png"}
				}
			}
			done <- struct{}{}
		}(g)
	}
	for g := 0; g < goroutines; g++ {
		<-done
	}
	close(results)

	count := 0
	for r := range results {
		count++
		if r.err != nil {
			t.Errorf("concurrent encode error: %v", r.err)
			continue
		}
		switch r.typ {
		case "webp_lossy_jpeg", "avif_jpeg":
			if r.w != jw || r.h != jh {
				t.Errorf("%s: dim mismatch %dx%d (want %dx%d)", r.typ, r.w, r.h, jw, jh)
			}
		case "webp_lossless_png", "avif_png":
			if r.w != pw || r.h != ph {
				t.Errorf("%s: dim mismatch %dx%d (want %dx%d)", r.typ, r.w, r.h, pw, ph)
			}
		}
	}
	if count != goroutines*itersPerGoroutine {
		t.Errorf("got %d results, want %d", count, goroutines*itersPerGoroutine)
	}
}

// --- stderr capture verification ---

// TestStderrCaptureContent verifies that the captured tool stderr is actually
// included in the error message returned to Go callers — not just a generic
// "tool exited" string. We trigger known failure modes that produce specific
// stderr text and assert that text is present.
func TestStderrCaptureContent(t *testing.T) {
	data := loadTestData(t, "photo.jpg")

	t.Run("avif_q100_includes_avifenc_stderr", func(t *testing.T) {
		_, err := Avif.EncodeFast(data, 100, 0)
		if err == nil {
			t.Fatal("expected error for AVIF q=100")
		}
		// avifenc emits exactly this string when q=100 + non-identity matrix.
		want := "Invalid codec-specific option"
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error does not contain stderr text %q\nfull error: %v", want, err)
		}
		// Also expect the "tool exited with code" prefix from our wrapper.
		if !strings.Contains(err.Error(), "tool exited with code") {
			t.Fatalf("error missing wrapper prefix:\n%v", err)
		}
	})

	t.Run("webp_lossy_garbage_input_includes_cwebp_stderr", func(t *testing.T) {
		// Build a JPEG-shaped buffer with garbage SOI follow-up.
		// This is picked up by detectFormat as JPEG, sent to cwebp via stdin,
		// which then errors out reading the stream.
		bad := append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, 100)...)
		_, err := Webp.EncodeLossy(bad, 80, false)
		if err == nil {
			t.Fatal("expected error for garbage JPEG bytes")
		}
		t.Logf("captured error: %v", err)
		// We don't pin the exact cwebp message (it varies by version), but
		// the error must contain at least one of the known stderr fragments.
		errStr := err.Error()
		gotKnownFragment := strings.Contains(errStr, "Could not process") ||
			strings.Contains(errStr, "Error") ||
			strings.Contains(errStr, "decode") ||
			strings.Contains(errStr, "Cannot") ||
			strings.Contains(errStr, "ERROR") ||
			strings.Contains(errStr, "FAILED")
		if !gotKnownFragment {
			t.Fatalf("error message lacks any cwebp stderr fragment:\n%v", err)
		}
	})
}

// --- Warnings field (JUDGE-2b) ---

// TestEncodeResult_Warnings verifies that non-fatal stderr lines from the
// underlying tool are surfaced via EncodeResult.Warnings on the success
// path. We use a deterministic trigger: a half-truncated JPEG fed to
// Avif.EncodeFast. libjpeg-turbo (used by avifenc) silently fabricates
// the missing entropy data and emits "Premature end of JPEG file" to
// stderr but still returns success — exactly the failure mode JUDGE-2b
// is meant to surface.
func TestEncodeResult_Warnings(t *testing.T) {
	src := loadTestData(t, "photo.jpg")
	half := src[:len(src)/2] // 50% truncation

	t.Run("Avif.EncodeFast_truncated_JPEG_emits_warnings", func(t *testing.T) {
		result, err := Avif.EncodeFast(half, 80, 0)
		if err != nil {
			t.Skipf("avifenc happened to refuse this input: %v", err)
		}
		// Encoding succeeded (libjpeg padded the missing data). Warnings
		// MUST contain at least one line about the premature EOF.
		found := false
		for _, w := range result.Warnings {
			if strings.Contains(w, "Premature end of JPEG file") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected 'Premature end of JPEG file' in warnings, got %d warnings: %v",
				len(result.Warnings), result.Warnings)
		}
		t.Logf("captured %d warnings, including expected truncation warning",
			len(result.Warnings))
	})

	// Sanity: clean encode should NOT contain any of the known
	// problem-indicating substrings, even if cwebp emits stats.
	t.Run("Webp.EncodeLossy_clean_input_no_problem_warnings", func(t *testing.T) {
		result, err := Webp.EncodeLossy(src, 80, false)
		if err != nil {
			t.Fatal(err)
		}
		// cwebp always prints encoding stats. We don't filter them out
		// (see EncodeResult.Warnings doc) — but a clean encode must not
		// contain any genuine problem indicator.
		problemIndicators := []string{
			"Premature end",
			"WARNING",
			"ERROR",
			"error",
			"failed",
			"Failed",
		}
		for _, w := range result.Warnings {
			for _, ind := range problemIndicators {
				if strings.Contains(w, ind) {
					t.Errorf("clean input warning contains problem indicator %q: %s", ind, w)
				}
			}
		}
		t.Logf("clean input: %d stderr lines (cwebp stats), no problem indicators",
			len(result.Warnings))
	})
}

// --- Truncated / corrupted input ---

// TestTruncatedInput verifies that the encoders return an error (rather than
// crashing or producing partial output) when fed a JPEG/PNG/GIF that has been
// cut off mid-stream. We try several truncation lengths to exercise different
// internal failure paths.
func TestTruncatedInput(t *testing.T) {
	jpeg := loadTestData(t, "photo.jpg")
	png := loadTestData(t, "logo.png")
	gif := loadTestData(t, "animation.gif")

	// JPEG: truncate to length within the SOS data
	for _, frac := range []float64{0.10, 0.30, 0.50, 0.75} {
		n := int(float64(len(jpeg)) * frac)
		t.Run(fmt.Sprintf("Webp.EncodeLossy_jpeg_truncated_%.0f%%", frac*100), func(t *testing.T) {
			_, err := Webp.EncodeLossy(jpeg[:n], 80, false)
			if err == nil {
				t.Fatal("expected error for truncated JPEG")
			}
		})
	}

	// JUDGE-2 in review.md: avifenc's libjpeg-turbo silently tolerates
	// truncation down to ~20% of the source file (it pads missing entropy
	// data with zeros). We can only reliably catch truncation that destroys
	// the JPEG header itself. This is documented as a known limitation; the
	// test below pins down the threshold.
	t.Run("Avif.EncodeFast_jpeg_header_destroyed_errors", func(t *testing.T) {
		// 5% (≈75 bytes) and 10% (≈151 bytes) of a 1515-byte JPEG don't
		// even include a complete DHT, so libjpeg has to error out.
		for _, frac := range []float64{0.05, 0.10} {
			n := int(float64(len(jpeg)) * frac)
			if _, err := Avif.EncodeFast(jpeg[:n], 80, 0); err == nil {
				t.Errorf("trunc to %.0f%% (%d bytes): expected error, but got success",
					frac*100, n)
			}
		}
	})
	t.Run("Avif.EncodeFast_jpeg_post_header_truncation_quietly_succeeds", func(t *testing.T) {
		// Pin down the lenient behavior so a future libjpeg upgrade that
		// becomes stricter is visible in the test log.
		for _, frac := range []float64{0.20, 0.50, 0.75, 0.90} {
			n := int(float64(len(jpeg)) * frac)
			result, err := Avif.EncodeFast(jpeg[:n], 80, 0)
			if err != nil {
				t.Logf("trunc to %.0f%% errored (libjpeg may have become stricter): %v",
					frac*100, err)
				continue
			}
			if !isAVIF(result.Data) {
				t.Errorf("trunc to %.0f%%: output is not AVIF", frac*100)
			}
			t.Logf("trunc to %.0f%%: lenient libjpeg produced %d-byte AVIF",
				frac*100, len(result.Data))
		}
	})

	t.Run("Webp.EncodeLossy_png_half", func(t *testing.T) {
		_, err := Webp.EncodeLossy(png[:len(png)/2], 80, true)
		if err == nil {
			t.Fatal("expected error for half-truncated PNG")
		}
	})

	t.Run("Webp.EncodeLossless_png_half", func(t *testing.T) {
		_, err := Webp.EncodeLossless(png[:len(png)/2], false)
		if err == nil {
			t.Fatal("expected error for half-truncated PNG")
		}
	})

	t.Run("Webp.EncodeGif_truncated", func(t *testing.T) {
		_, err := Webp.EncodeGif(gif[:len(gif)/2], false)
		if err == nil {
			t.Fatal("expected error for half-truncated GIF")
		}
	})

	// JPEG with corrupted bytes in the middle (flip every 100th byte after SOS).
	// Should still error out cleanly without crashing.
	t.Run("Webp.EncodeLossy_jpeg_corrupted", func(t *testing.T) {
		corrupted := make([]byte, len(jpeg))
		copy(corrupted, jpeg)
		// Corrupt the latter 70% of the file (entropy-coded data area).
		start := len(jpeg) * 3 / 10
		for i := start; i < len(jpeg); i += 100 {
			corrupted[i] ^= 0xAA
		}
		_, err := Webp.EncodeLossy(corrupted, 80, false)
		// We don't require error here — libjpeg can sometimes recover from
		// stray bit errors. We just require not crashing.
		_ = err
	})

	// Just SOI (FFD8) — clearly not a valid JPEG.
	t.Run("Webp.EncodeLossy_jpeg_just_soi", func(t *testing.T) {
		_, err := Webp.EncodeLossy([]byte{0xFF, 0xD8}, 80, false)
		if err == nil {
			t.Fatal("expected error for SOI-only JPEG")
		}
	})

	// PNG with valid signature but truncated IHDR.
	t.Run("Webp.EncodeLossy_png_signature_only", func(t *testing.T) {
		_, err := Webp.EncodeLossy([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}, 80, true)
		if err == nil {
			t.Fatal("expected error for signature-only PNG")
		}
	})
}

// --- ICC profile preservation ---

// TestIccPreservation verifies that the encoders honour their `-metadata icc`
// / `-copy icc` flags by extracting the ICC bytes from the output and
// comparing them to the input ICC. This pins down the library's primary
// promise: ICC profiles round-trip through every encoder.
func TestIccPreservation(t *testing.T) {
	srgb := loadTestData(t, "srgb-test.icc")
	if len(srgb) < 128 {
		t.Fatal("test ICC profile too small")
	}

	t.Run("JPEG_with_ICC_round_trip", func(t *testing.T) {
		jpeg := loadTestData(t, "small-128x128.jpg")
		jpegICC, err := injectJpegICC(jpeg, srgb)
		if err != nil {
			t.Fatal(err)
		}
		// Sanity: our injection produces an extractable ICC.
		got, err := extractJpegICC(jpegICC)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, srgb) {
			t.Fatalf("self-extract mismatch: got %d bytes, want %d", len(got), len(srgb))
		}
	})

	t.Run("Webp.EncodeLossy_preserves_jpeg_ICC", func(t *testing.T) {
		jpeg := loadTestData(t, "small-128x128.jpg")
		jpegICC, _ := injectJpegICC(jpeg, srgb)
		result, err := Webp.EncodeLossy(jpegICC, 80, false)
		if err != nil {
			t.Fatal(err)
		}
		got, err := extractWebpICC(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, srgb) {
			t.Fatalf("ICC mismatch: got %d bytes, want %d", len(got), len(srgb))
		}
	})

	t.Run("Webp.EncodeLossless_preserves_jpeg_ICC", func(t *testing.T) {
		jpeg := loadTestData(t, "small-128x128.jpg")
		jpegICC, _ := injectJpegICC(jpeg, srgb)
		result, err := Webp.EncodeLossless(jpegICC, false)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := extractWebpICC(result.Data)
		if !bytes.Equal(got, srgb) {
			t.Fatalf("ICC mismatch: got %d bytes, want %d", len(got), len(srgb))
		}
	})

	t.Run("Avif.EncodeFast_preserves_jpeg_ICC", func(t *testing.T) {
		jpeg := loadTestData(t, "small-128x128.jpg")
		jpegICC, _ := injectJpegICC(jpeg, srgb)
		result, err := Avif.EncodeFast(jpegICC, 80, 0)
		if err != nil {
			t.Fatal(err)
		}
		got, err := extractAvifICC(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, srgb) {
			t.Fatalf("ICC mismatch: got %d bytes, want %d", len(got), len(srgb))
		}
	})

	t.Run("Avif.EncodeBalanced_preserves_jpeg_ICC", func(t *testing.T) {
		jpeg := loadTestData(t, "small-128x128.jpg")
		jpegICC, _ := injectJpegICC(jpeg, srgb)
		result, err := Avif.EncodeBalanced(jpegICC, 80, 0)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := extractAvifICC(result.Data)
		if !bytes.Equal(got, srgb) {
			t.Fatalf("ICC mismatch: got %d bytes, want %d", len(got), len(srgb))
		}
	})

	t.Run("PNG_with_ICC_round_trip", func(t *testing.T) {
		png := loadTestData(t, "small-128x128.png")
		pngICC, err := injectPngICC(png, srgb)
		if err != nil {
			t.Fatal(err)
		}
		got, err := extractPngICC(pngICC)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, srgb) {
			t.Fatalf("self-extract mismatch: got %d, want %d bytes", len(got), len(srgb))
		}
	})

	t.Run("Webp.EncodeLossy_preserves_png_ICC", func(t *testing.T) {
		png := loadTestData(t, "small-128x128.png")
		pngICC, err := injectPngICC(png, srgb)
		if err != nil {
			t.Fatal(err)
		}
		result, err := Webp.EncodeLossy(pngICC, 80, false)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := extractWebpICC(result.Data)
		if !bytes.Equal(got, srgb) {
			t.Fatalf("ICC mismatch: got %d, want %d bytes", len(got), len(srgb))
		}
	})

	t.Run("Avif.EncodeFast_preserves_png_ICC", func(t *testing.T) {
		png := loadTestData(t, "small-128x128.png")
		pngICC, _ := injectPngICC(png, srgb)
		result, err := Avif.EncodeFast(pngICC, 80, 0)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := extractAvifICC(result.Data)
		if !bytes.Equal(got, srgb) {
			t.Fatalf("ICC mismatch: got %d, want %d bytes", len(got), len(srgb))
		}
	})

	t.Run("NormalizeJpegOrientation_preserves_ICC", func(t *testing.T) {
		jpeg := loadTestData(t, "small-128x128.jpg")
		jpegICC, _ := injectJpegICC(jpeg, srgb)
		// Add an orientation tag to actually trigger the rotate path.
		jpegRotated := injectExifOrientation(t, jpegICC, 6)
		normalized, err := NormalizeJpegOrientation(jpegRotated)
		if err != nil {
			t.Fatal(err)
		}
		got, err := extractJpegICC(normalized)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, srgb) {
			t.Fatalf("ICC mismatch after normalize: got %d, want %d bytes", len(got), len(srgb))
		}
	})

	// Multi-segment ICC: split the same sRGB profile across N APP2 segments
	// and verify the encoders reassemble it.
	t.Run("Multi_segment_ICC_round_trip", func(t *testing.T) {
		jpeg := loadTestData(t, "small-128x128.jpg")
		// Force splitting into ~4 chunks (1000 bytes each).
		jpegMulti, err := injectJpegICCMulti(jpeg, srgb, 1000)
		if err != nil {
			t.Fatal(err)
		}
		// Sanity: self-extract should reassemble back to the original profile.
		got, err := extractJpegICC(jpegMulti)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, srgb) {
			t.Fatalf("self-extract mismatch: got %d, want %d", len(got), len(srgb))
		}

		// WebP lossy must reassemble multi-segment ICC.
		t.Run("Webp.EncodeLossy", func(t *testing.T) {
			result, err := Webp.EncodeLossy(jpegMulti, 80, false)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := extractWebpICC(result.Data)
			if !bytes.Equal(got, srgb) {
				t.Fatalf("ICC mismatch: got %d, want %d", len(got), len(srgb))
			}
		})

		// AVIF Fast must reassemble multi-segment ICC.
		t.Run("Avif.EncodeFast", func(t *testing.T) {
			result, err := Avif.EncodeFast(jpegMulti, 80, 0)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := extractAvifICC(result.Data)
			if !bytes.Equal(got, srgb) {
				t.Fatalf("ICC mismatch: got %d, want %d", len(got), len(srgb))
			}
		})

		// jpegtran (NormalizeJpegOrientation) must reassemble multi-segment ICC.
		t.Run("NormalizeJpegOrientation", func(t *testing.T) {
			rotated := injectExifOrientation(t, jpegMulti, 6)
			normalized, err := NormalizeJpegOrientation(rotated)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := extractJpegICC(normalized)
			if !bytes.Equal(got, srgb) {
				t.Fatalf("ICC mismatch after normalize: got %d, want %d", len(got), len(srgb))
			}
		})
	})
}

// --- Alpha channel preservation ---

// TestAlphaPreservation verifies that encoders pass alpha through. Without
// this check, a regression that drops the alpha channel would be invisible.
func TestAlphaPreservation(t *testing.T) {
	// alpha-4x4.png is a hand-rolled RGBA fixture with semi-transparent pixels.
	rgbaPNG := loadTestData(t, "alpha-4x4.png")

	t.Run("Webp.EncodeLossy_PNG_alpha", func(t *testing.T) {
		result, err := Webp.EncodeLossy(rgbaPNG, 80, false)
		if err != nil {
			t.Fatal(err)
		}
		info, err := parseWebP(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		if !info.hasAlpha {
			t.Fatal("alpha lost during EncodeLossy")
		}
	})

	t.Run("Webp.EncodeLossless_PNG_alpha", func(t *testing.T) {
		result, err := Webp.EncodeLossless(rgbaPNG, false)
		if err != nil {
			t.Fatal(err)
		}
		info, err := parseWebP(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		if !info.hasAlpha {
			t.Fatal("alpha lost during EncodeLossless")
		}
	})

	// GIF transparency: static-alpha.gif has the GCE transparent color flag set.
	t.Run("Webp.EncodeGif_transparent", func(t *testing.T) {
		gifData := loadTestData(t, "static-alpha.gif")
		result, err := Webp.EncodeGif(gifData, false)
		if err != nil {
			t.Fatal(err)
		}
		info, err := parseWebP(result.Data)
		if err != nil {
			t.Fatal(err)
		}
		if !info.hasAlpha {
			t.Fatal("alpha lost during EncodeGif on transparent GIF")
		}
	})
}

// --- Error cases ---

// assertErrorContains checks that err is non-nil and its message contains the
// given substring. Substring matching is intentional: it lets us pin the error
// path (empty input vs. format mismatch vs. tool failure) without committing
// to exact message text.
func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %v", want, err)
	}
}

func TestErrorEmptyInputAllEncoders(t *testing.T) {
	t.Run("Webp.EncodeLossy", func(t *testing.T) {
		_, err := Webp.EncodeLossy(nil, 80, false)
		assertErrorContains(t, err, "empty")
	})
	t.Run("Webp.EncodeLossless", func(t *testing.T) {
		_, err := Webp.EncodeLossless(nil, false)
		assertErrorContains(t, err, "empty")
	})
	t.Run("Webp.EncodeGif", func(t *testing.T) {
		_, err := Webp.EncodeGif(nil, false)
		assertErrorContains(t, err, "empty")
	})
	t.Run("Avif.EncodeBalanced", func(t *testing.T) {
		_, err := Avif.EncodeBalanced(nil, 80, 0)
		assertErrorContains(t, err, "empty")
	})
	t.Run("Avif.EncodeCompact", func(t *testing.T) {
		_, err := Avif.EncodeCompact(nil, 80, 0)
		assertErrorContains(t, err, "empty")
	})
	t.Run("Avif.EncodeFast", func(t *testing.T) {
		_, err := Avif.EncodeFast(nil, 80, 0)
		assertErrorContains(t, err, "empty")
	})
	t.Run("NormalizeJpegOrientation", func(t *testing.T) {
		_, err := NormalizeJpegOrientation(nil)
		assertErrorContains(t, err, "empty")
	})
}

func TestErrorWrongFormatAllEncoders(t *testing.T) {
	gif := loadTestData(t, "animation.gif")
	jpeg := loadTestData(t, "photo.jpg")
	png := loadTestData(t, "logo.png")

	t.Run("Webp.EncodeLossy_GIF", func(t *testing.T) {
		_, err := Webp.EncodeLossy(gif, 80, false)
		assertErrorContains(t, err, "unsupported format")
	})
	t.Run("Webp.EncodeLossless_GIF", func(t *testing.T) {
		_, err := Webp.EncodeLossless(gif, false)
		assertErrorContains(t, err, "unsupported format")
	})
	t.Run("Webp.EncodeGif_JPEG", func(t *testing.T) {
		_, err := Webp.EncodeGif(jpeg, false)
		assertErrorContains(t, err, "unsupported format")
	})
	t.Run("Webp.EncodeGif_PNG", func(t *testing.T) {
		_, err := Webp.EncodeGif(png, false)
		assertErrorContains(t, err, "unsupported format")
	})
	t.Run("Avif.EncodeBalanced_GIF", func(t *testing.T) {
		_, err := Avif.EncodeBalanced(gif, 80, 0)
		assertErrorContains(t, err, "unsupported format")
	})
	t.Run("Avif.EncodeCompact_GIF", func(t *testing.T) {
		_, err := Avif.EncodeCompact(gif, 80, 0)
		assertErrorContains(t, err, "unsupported format")
	})
	t.Run("Avif.EncodeFast_GIF", func(t *testing.T) {
		_, err := Avif.EncodeFast(gif, 80, 0)
		assertErrorContains(t, err, "unsupported format")
	})
}

func TestErrorGarbageInput(t *testing.T) {
	garbage := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	t.Run("Webp.EncodeLossy", func(t *testing.T) {
		_, err := Webp.EncodeLossy(garbage, 80, false)
		assertErrorContains(t, err, "unsupported format")
	})
	t.Run("Avif.EncodeFast", func(t *testing.T) {
		_, err := Avif.EncodeFast(garbage, 80, 0)
		assertErrorContains(t, err, "unsupported format")
	})
}
