// Helper to generate testdata/marker-80x48.jpg — a JPEG with a single bright
// red marker block at an asymmetric position. Used by P12-1 to verify the
// direction of jpegtran's lossless rotations end-to-end.
//
// Layout:
//   - 80 wide × 48 tall (both multiples of 16, so jpegtran -trim is a no-op)
//   - Dark gray background
//   - Bright red 4×4 block at position (8, 4) in stored coordinates
//     → marker centroid at (10, 6)
//
// After each EXIF orientation transform, the centroid maps to a unique
// position in the rotated image — see the test for the 8 expected mappings.
//
// Run with `go run ./scripts/gen_marker_jpeg` from the repo root.
package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
)

func main() {
	const (
		w     = 80
		h     = 48
		mx    = 8 // marker top-left x
		my    = 4 // marker top-left y
		msize = 4 // marker block size
	)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{R: 32, G: 32, B: 32, A: 255}     // dark gray background
	mk := color.RGBA{R: 255, G: 0, B: 0, A: 255}       // bright red marker
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, bg)
		}
	}
	for y := my; y < my+msize; y++ {
		for x := mx; x < mx+msize; x++ {
			img.Set(x, y, mk)
		}
	}

	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
	out := filepath.Join(repoRoot, "testdata", "marker-80x48.jpg")
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	// High quality so the bright red marker survives lossy compression cleanly.
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 95}); err != nil {
		panic(err)
	}
}
