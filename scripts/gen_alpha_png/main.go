// Helper to generate testdata/alpha-4x4.png — a tiny RGBA PNG with
// semi-transparent pixels. Run with `go run ./scripts/gen_alpha_png`
// from the repo root, or from any cwd (the script resolves the testdata
// path relative to its own source file).
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
)

func main() {
	const w, h = 4, 4
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := (x + y) * 64
			if a > 255 {
				a = 255
			}
			img.Set(x, y, color.NRGBA{R: 255, G: uint8(x * 64), B: uint8(y * 64), A: uint8(a)})
		}
	}

	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..")
	out := filepath.Join(repoRoot, "testdata", "alpha-4x4.png")
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
}
