// Helper to generate testdata/nonmcu-99x97.jpg — a JPEG with non-MCU-aligned
// dimensions (99 wide, 97 tall, neither a multiple of 16). Used to exercise
// the jpegtran -trim path during EXIF orientation normalization.
// Run with `go run ./scripts/gen_nonmcu_jpeg` from the repo root.
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
	const w, h = 99, 97
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / (w - 1)),
				G: uint8((y * 255) / (h - 1)),
				B: uint8(((x + y) * 255) / (w + h - 2)),
				A: 255,
			})
		}
	}

	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
	out := filepath.Join(repoRoot, "testdata", "nonmcu-99x97.jpg")
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
}
