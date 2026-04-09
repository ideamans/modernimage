// Decode → rotate → re-encode fallback for NormalizeJpegOrientation.
//
// Used when jpegtran -perfect refuses an input because the source JPEG was
// encoded without iMCU-aligned padding (some non-libjpeg encoders do this).
// The cost is a JPEG re-encode quality round trip; the benefit is that no
// edge pixels are dropped, even when jpegtran's lossless transform is
// impossible.
//
// The original input's ICC profile (if any) is extracted before decoding
// and re-injected into the encoded output, so color management round-trips.

package modernimage

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
)

// Quality used for the fallback re-encode. Chosen high enough that the
// rotation round trip is visually indistinguishable from the source for
// typical photographic input. The user explicitly accepted "時間がかかっても
// 欠けることがないように" — pixel completeness over speed/size.
const fallbackJpegQuality = 95

func normalizeViaDecodeRotateEncode(data []byte, orientation int) ([]byte, error) {
	// Step 1: extract the ICC profile (so we can re-inject it after the
	// re-encode strips it).
	icc, err := extractJpegICC(data)
	if err != nil {
		// Non-fatal: a malformed APP2 doesn't prevent us from rotating.
		icc = nil
	}

	// Step 2: decode the JPEG.
	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	// Step 3: apply the orientation transform.
	rotated := rotateImageForOrientation(src, orientation)

	// Step 4: re-encode as JPEG.
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rotated, &jpeg.Options{Quality: fallbackJpegQuality}); err != nil {
		return nil, fmt.Errorf("re-encode: %w", err)
	}
	out := buf.Bytes()

	// Step 5: re-inject the ICC profile if we had one.
	if len(icc) > 0 {
		withICC, err := injectJpegICC(out, icc)
		if err != nil {
			// If the ICC happens to be too large for a single APP2 segment,
			// fall through and return the result without ICC rather than
			// failing the whole operation. This is a soft failure.
			return out, nil
		}
		return withICC, nil
	}
	return out, nil
}

// rotateImageForOrientation returns a new image where the source pixels have
// been transformed according to the EXIF orientation value (1-8). The
// returned image is always an *image.RGBA so encoding is uniform regardless
// of the source's color model.
func rotateImageForOrientation(src image.Image, orientation int) image.Image {
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	// For each EXIF orientation, define the destination dimensions and a
	// pixel-coordinate map from src (x, y) to dst (nx, ny).
	var newW, newH int
	var pixelMap func(x, y int) (int, int)
	switch orientation {
	case 2: // mirror horizontal
		newW, newH = w, h
		pixelMap = func(x, y int) (int, int) { return w - 1 - x, y }
	case 3: // 180°
		newW, newH = w, h
		pixelMap = func(x, y int) (int, int) { return w - 1 - x, h - 1 - y }
	case 4: // mirror vertical
		newW, newH = w, h
		pixelMap = func(x, y int) (int, int) { return x, h - 1 - y }
	case 5: // transpose (flip across main diagonal)
		newW, newH = h, w
		pixelMap = func(x, y int) (int, int) { return y, x }
	case 6: // 90° CW
		newW, newH = h, w
		pixelMap = func(x, y int) (int, int) { return h - 1 - y, x }
	case 7: // transverse (flip across secondary diagonal)
		newW, newH = h, w
		pixelMap = func(x, y int) (int, int) { return h - 1 - y, w - 1 - x }
	case 8: // 270° CW
		newW, newH = h, w
		pixelMap = func(x, y int) (int, int) { return y, w - 1 - x }
	default:
		// orientation 1 (or unknown) — return a copy converted to RGBA.
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(x, y, src.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
		return dst
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			nx, ny := pixelMap(x, y)
			dst.Set(nx, ny, color.RGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)))
		}
	}
	return dst
}
