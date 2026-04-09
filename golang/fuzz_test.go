// Fuzz targets for parsers that handle untrusted input.
//
// Run with:
//   go test -run='^$' -fuzz=FuzzJpegOrientation -fuzztime=30s
//   go test -run='^$' -fuzz=FuzzImageDimensions -fuzztime=30s
//
// The targets only assert "no crash, no panic, no infinite loop". They are
// expected to handle arbitrary bytes safely.

package modernimage

import (
	"os"
	"testing"
)

// FuzzJpegOrientation feeds random bytes to the public API JpegOrientation,
// which goes through CGO into modernimage_jpeg_orientation in C. The C
// implementation is a hand-rolled binary parser of JPEG APP1/EXIF segments,
// so it is the most likely place for an out-of-bounds read.
//
// The function must return a value in [0, 8] and never crash.
func FuzzJpegOrientation(f *testing.F) {
	// Seed corpus from real fixtures.
	for _, name := range []string{
		"small-128x128.jpg",
		"photo.jpg",
		"landscape-like.jpg",
		"exif6-real.jpg",
	} {
		data, err := readTestData(name)
		if err == nil {
			f.Add(data)
		}
	}
	// A few hand-crafted edge cases.
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xD8})                                   // SOI only
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x02})           // empty APP1
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0xFF, 0xFF})           // APP1 with bogus length
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x08, 'E', 'x', 'i', 'f', 0x00, 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		ori := JpegOrientation(data)
		if ori < 0 || ori > 8 {
			t.Fatalf("orientation out of range [0, 8]: %d (input %d bytes)", ori, len(data))
		}
	})
}

// FuzzImageDimensions exercises the test-only multi-format parser used by
// the dimension assertions throughout the test suite. While not a public
// API, it must still handle untrusted bytes safely (no panic, no
// out-of-bounds slice access) so the test infrastructure itself is
// trustworthy.
func FuzzImageDimensions(f *testing.F) {
	for _, name := range []string{
		"small-128x128.jpg",
		"logo.png",
		"animation.gif",
		"alpha-4x4.png",
	} {
		if data, err := readTestData(name); err == nil {
			f.Add(data)
		}
	}
	// Crafted: each format magic with truncated payload
	f.Add([]byte{0xFF, 0xD8, 0xFF})                              // JPEG header
	f.Add([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'})   // PNG signature
	f.Add([]byte{'G', 'I', 'F', '8', '9', 'a'})                  // GIF signature
	f.Add([]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}) // WebP minimal
	f.Add([]byte{0, 0, 0, 0x10, 'f', 't', 'y', 'p'})             // ftyp box

	f.Fuzz(func(t *testing.T, data []byte) {
		// imageDimensions returns (w, h, error). We don't care about the
		// values — only that the function doesn't panic.
		w, h, err := imageDimensions(data)
		if err == nil {
			if w < 0 || h < 0 {
				t.Fatalf("negative dims %dx%d for %d-byte input", w, h, len(data))
			}
		}
	})
}

// readTestData is a non-test-helper version that returns an error instead of
// failing the test (so it can be called from FuzzXxx setup functions).
func readTestData(name string) ([]byte, error) {
	return os.ReadFile("../testdata/" + name)
}
