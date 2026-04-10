// Package modernimage provides Go bindings for libmodernimage,
// offering simplified WebP and AVIF encoding functions.
package modernimage

/*
#cgo CFLAGS: -I${SRCDIR}/shared/include

// Platform-specific static libraries (all dependencies bundled in .a)
#cgo darwin,arm64 LDFLAGS: ${SRCDIR}/shared/lib/darwin-arm64/libmodernimage.a
#cgo darwin,amd64 LDFLAGS: ${SRCDIR}/shared/lib/darwin-amd64/libmodernimage.a
#cgo linux,amd64 LDFLAGS: ${SRCDIR}/shared/lib/linux-amd64/libmodernimage.a
#cgo linux,arm64 LDFLAGS: ${SRCDIR}/shared/lib/linux-arm64/libmodernimage.a

#cgo darwin LDFLAGS: -lc++ -lpthread -lm
#cgo linux LDFLAGS: -lstdc++ -lpthread -lm

#include <stdlib.h>
#include "modernimage.h"
*/
import "C"
import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"
)

// EncodeResult holds the output of any encode function.
//
// Warnings contains every non-empty stderr line emitted by the underlying
// tool (cwebp / avifenc / gif2webp / jpegtran). The encode operation
// succeeded — these are not fatal. The slice may contain a mix of:
//
//   - Genuine warnings about data quality issues, e.g.
//     "Premature end of JPEG file" (libjpeg quietly fabricated missing
//     entropy data — your output likely has black/grey areas where the
//     source was truncated)
//   - Informational diagnostics that the tool always prints (cwebp in
//     particular prints encoding stats: dimensions, output size, PSNR,
//     macroblock distribution, etc.)
//
// libmodernimage does NOT filter these; the caller is expected to inspect
// the lines and decide which deserve attention. A safe heuristic is to
// look for known warning substrings ("Premature end", "WARNING", "ERROR"
// etc.) rather than treating every line as a problem.
//
// Empty when the tool produced no stderr.
type EncodeResult struct {
	Data     []byte
	MimeType string
	Warnings []string
}

// detectFormat returns "jpeg", "png", "gif", or "" based on magic bytes.
func detectFormat(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpeg"
	}
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "png"
	}
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
		return "gif"
	}
	return ""
}

type toolKind int

const (
	toolCwebp    toolKind = 0
	toolGif2webp toolKind = 1
	toolAvifenc  toolKind = 2
	toolJpegtran toolKind = 3
)

// callTool handles context lifecycle, temp files, stdin injection, tool execution, and output reading.
func callTool(tool toolKind, inputData []byte, argv []string) (*EncodeResult, error) {
	ctx := C.modernimage_context_new()
	if ctx == nil {
		return nil, fmt.Errorf("modernimage: failed to create context")
	}
	defer C.modernimage_context_free(ctx)

	// Pin input data and set stdin for cwebp/avifenc
	if tool != toolGif2webp {
		var pinner runtime.Pinner
		pinner.Pin(&inputData[0])
		defer pinner.Unpin()
		C.modernimage_set_stdin(ctx, unsafe.Pointer(&inputData[0]), C.size_t(len(inputData)))
	}

	// Build C argv
	argc := C.int(len(argv))
	cArgv := make([]*C.char, len(argv))
	for i, s := range argv {
		cArgv[i] = C.CString(s)
	}
	defer func() {
		for _, p := range cArgv {
			C.free(unsafe.Pointer(p))
		}
	}()

	argvPtr := (**C.char)(unsafe.Pointer(&cArgv[0]))

	// Execute
	var rc C.int
	switch tool {
	case toolCwebp:
		rc = C.modernimage_cwebp(ctx, argc, argvPtr)
	case toolGif2webp:
		rc = C.modernimage_gif2webp(ctx, argc, argvPtr)
	case toolAvifenc:
		rc = C.modernimage_avifenc(ctx, argc, argvPtr)
	case toolJpegtran:
		rc = C.modernimage_jpegtran(ctx, argc, argvPtr)
	}

	if rc != 0 {
		errSize := C.modernimage_get_stderr_size(ctx)
		if errSize > 0 {
			errBuf := make([]byte, errSize)
			C.modernimage_copy_stderr(ctx, (*C.char)(unsafe.Pointer(&errBuf[0])), errSize)
			return nil, fmt.Errorf("modernimage: tool exited with code %d: %s", int(rc), string(errBuf))
		}
		return nil, fmt.Errorf("modernimage: tool exited with code %d", int(rc))
	}

	// Find and read the output file (last arg after -o or -outfile)
	outPath := ""
	for i, a := range argv {
		if (a == "-o" || a == "-outfile") && i+1 < len(argv) {
			outPath = argv[i+1]
			break
		}
	}
	if outPath == "" {
		return nil, fmt.Errorf("modernimage: no output path found in argv")
	}

	outData, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("modernimage: failed to read output: %w", err)
	}
	if len(outData) == 0 {
		return nil, fmt.Errorf("modernimage: encoding produced empty output")
	}

	// Capture non-fatal stderr from the success path. These become
	// EncodeResult.Warnings so callers can detect silent data quality
	// issues without having to fail.
	warnings := captureStderrWarnings(ctx)

	return &EncodeResult{Data: outData, Warnings: warnings}, nil
}

// captureStderrWarnings reads any stderr captured by the C context and
// returns it as a slice of non-empty trimmed lines.
func captureStderrWarnings(ctx *C.modernimage_context_t) []string {
	errSize := C.modernimage_get_stderr_size(ctx)
	if errSize == 0 {
		return nil
	}
	errBuf := make([]byte, errSize)
	C.modernimage_copy_stderr(ctx, (*C.char)(unsafe.Pointer(&errBuf[0])), errSize)
	raw := string(errBuf)
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// createTempFile creates a temp file with the given suffix and returns its path.
func createTempFile(suffix string) (string, error) {
	f, err := os.CreateTemp("", "modernimage-*"+suffix)
	if err != nil {
		return "", err
	}
	path := f.Name()
	f.Close()
	return path, nil
}

// WebpEncoder groups WebP encoding functions.
type WebpEncoder struct{}

// AvifEncoder groups AVIF encoding functions.
type AvifEncoder struct{}

// Webp provides WebP encoding functions.
var Webp WebpEncoder

// Avif provides AVIF encoding functions.
var Avif AvifEncoder

// EncodeLossy encodes JPEG/PNG image data to lossy WebP.
// quality: 0-100 (default 80), multithread: enable multi-threaded encoding.
// ICC profiles are preserved from the source image.
func (WebpEncoder) EncodeLossy(data []byte, quality int, multithread bool) (*EncodeResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("modernimage: empty input data")
	}
	format := detectFormat(data)
	if format != "jpeg" && format != "png" {
		return nil, fmt.Errorf("modernimage: unsupported format for EncodeLossy (expected JPEG or PNG, got %q)", format)
	}

	tmpOut, err := createTempFile(".webp")
	if err != nil {
		return nil, fmt.Errorf("modernimage: %w", err)
	}
	defer os.Remove(tmpOut)

	argv := []string{"cwebp", "-q", fmt.Sprintf("%d", quality), "-metadata", "icc"}
	if multithread {
		argv = append(argv, "-mt")
	}
	argv = append(argv, "-o", tmpOut, "--", "-")

	result, err := callTool(toolCwebp, data, argv)
	if err != nil {
		return nil, err
	}
	result.MimeType = "image/webp"
	return result, nil
}

// EncodeLossless encodes JPEG/PNG image data to lossless WebP.
// multithread: enable multi-threaded encoding.
// ICC profiles are preserved from the source image.
func (WebpEncoder) EncodeLossless(data []byte, multithread bool) (*EncodeResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("modernimage: empty input data")
	}
	format := detectFormat(data)
	if format != "jpeg" && format != "png" {
		return nil, fmt.Errorf("modernimage: unsupported format for EncodeLossless (expected JPEG or PNG, got %q)", format)
	}

	tmpOut, err := createTempFile(".webp")
	if err != nil {
		return nil, fmt.Errorf("modernimage: %w", err)
	}
	defer os.Remove(tmpOut)

	argv := []string{"cwebp", "-lossless", "-metadata", "icc"}
	if multithread {
		argv = append(argv, "-mt")
	}
	argv = append(argv, "-o", tmpOut, "--", "-")

	result, err := callTool(toolCwebp, data, argv)
	if err != nil {
		return nil, err
	}
	result.MimeType = "image/webp"
	return result, nil
}

// EncodeGif encodes GIF image data to animated WebP.
// multithread: enable multi-threaded encoding.
func (WebpEncoder) EncodeGif(data []byte, multithread bool) (*EncodeResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("modernimage: empty input data")
	}
	format := detectFormat(data)
	if format != "gif" {
		return nil, fmt.Errorf("modernimage: unsupported format for EncodeGif (expected GIF, got %q)", format)
	}

	// gif2webp has no stdin support, write input to temp file
	tmpIn, err := createTempFile(".gif")
	if err != nil {
		return nil, fmt.Errorf("modernimage: %w", err)
	}
	defer os.Remove(tmpIn)
	if err := os.WriteFile(tmpIn, data, 0644); err != nil {
		return nil, fmt.Errorf("modernimage: failed to write temp input: %w", err)
	}

	tmpOut, err := createTempFile(".webp")
	if err != nil {
		return nil, fmt.Errorf("modernimage: %w", err)
	}
	defer os.Remove(tmpOut)

	argv := []string{"gif2webp"}
	if multithread {
		argv = append(argv, "-mt")
	}
	argv = append(argv, tmpIn, "-o", tmpOut)

	result, err := callTool(toolGif2webp, data, argv)
	if err != nil {
		return nil, err
	}
	result.MimeType = "image/webp"
	return result, nil
}

// AVIF preset parameters matching libnextimage-lite's avifenc_bridge.
// Each preset defines YUV format, CICP color properties, speed, threading, tiling, and tune.
type avifPreset struct {
	speed    int
	jobs     int    // default jobs (0 = auto/CPU count)
	yuv      string // "444" or "420"
	cicp     string // "CP/TC/MC" e.g. "1/1/1"
	depth    int    // bit depth (0 = default 8)
	autoTile bool
	tileRows int // tilerowslog2 (0 = not set)
	tileCols int // tilecolslog2 (0 = not set)
}

var (
	presetBalanced = avifPreset{
		speed: 6, jobs: 16, yuv: "444", cicp: "1/1/1",
		autoTile: true,
	}
	presetCompact = avifPreset{
		speed: 0, jobs: 0, yuv: "444", cicp: "1/1/1", depth: 10,
		autoTile: true,
	}
	presetFast = avifPreset{
		speed: 9, jobs: 16, yuv: "420", cicp: "1/13/6",
		tileRows: 6, tileCols: 6,
	}
)

// EncodeBalanced encodes JPEG/PNG image data to AVIF with balanced speed/quality.
// Uses YUV444, CICP 1/1/1 (BT.709), speed 6, autotiling, tune=ssimulacra2.
// quality: 0-100 (default 80), jobs: number of threads (0 = preset default 16).
func (AvifEncoder) EncodeBalanced(data []byte, quality int, jobs int) (*EncodeResult, error) {
	return encodeAvif(data, quality, jobs, presetBalanced, "EncodeBalanced")
}

// EncodeCompact encodes JPEG/PNG image data to AVIF with best compression (slowest).
// Uses YUV444, CICP 1/1/1 (BT.709), speed 0, 10-bit depth, autotiling, tune=ssimulacra2.
// quality: 0-100 (default 80), jobs: number of threads (0 = all CPUs).
func (AvifEncoder) EncodeCompact(data []byte, quality int, jobs int) (*EncodeResult, error) {
	return encodeAvif(data, quality, jobs, presetCompact, "EncodeCompact")
}

// EncodeFast encodes JPEG/PNG image data to AVIF with fastest speed.
// Uses YUV420, CICP 1/13/6 (BT.709/sRGB/BT.601), speed 9, 64x64 tiling, tune=ssimulacra2.
// quality: 0-100 (default 80), jobs: number of threads (0 = preset default 16).
func (AvifEncoder) EncodeFast(data []byte, quality int, jobs int) (*EncodeResult, error) {
	return encodeAvif(data, quality, jobs, presetFast, "EncodeFast")
}

func encodeAvif(data []byte, quality int, jobs int, preset avifPreset, opName string) (*EncodeResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("modernimage: empty input data")
	}
	format := detectFormat(data)
	if format != "jpeg" && format != "png" {
		return nil, fmt.Errorf("modernimage: unsupported format for %s (expected JPEG or PNG, got %q)", opName, format)
	}

	tmpOut, err := createTempFile(".avif")
	if err != nil {
		return nil, fmt.Errorf("modernimage: %w", err)
	}
	defer os.Remove(tmpOut)

	argv := []string{
		"avifenc",
		"-q", fmt.Sprintf("%d", quality),
		"-s", fmt.Sprintf("%d", preset.speed),
		"--yuv", preset.yuv,
		"--cicp", preset.cicp,
	}

	// Bit depth
	if preset.depth > 0 {
		argv = append(argv, "-d", fmt.Sprintf("%d", preset.depth))
	}

	// Threading
	effectiveJobs := preset.jobs
	if jobs > 0 {
		effectiveJobs = jobs
	} else if effectiveJobs == 0 {
		effectiveJobs = runtime.NumCPU()
	}
	argv = append(argv, "-j", fmt.Sprintf("%d", effectiveJobs))

	// Tiling
	if preset.autoTile {
		argv = append(argv, "--autotiling")
	}
	if preset.tileRows > 0 {
		argv = append(argv, "--tilerowslog2", fmt.Sprintf("%d", preset.tileRows))
	}
	if preset.tileCols > 0 {
		argv = append(argv, "--tilecolslog2", fmt.Sprintf("%d", preset.tileCols))
	}

	// Tune
	argv = append(argv, "-a", "tune=ssimulacra2")

	argv = append(argv, "--input-format", format, "-o", tmpOut, "--stdin")

	result, err := callTool(toolAvifenc, data, argv)
	if err != nil {
		return nil, err
	}
	result.MimeType = "image/avif"
	return result, nil
}

// JpegOrientation returns the EXIF orientation value (1-8) from JPEG data.
// Returns 0 if no orientation tag is found or data is not JPEG.
// This is a pure read-only operation — very fast, no decompression needed.
func JpegOrientation(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	var pinner runtime.Pinner
	pinner.Pin(&data[0])
	defer pinner.Unpin()
	return int(C.modernimage_jpeg_orientation(unsafe.Pointer(&data[0]), C.size_t(len(data))))
}

// NormalizeJpegOrientation applies rotation based on EXIF orientation, then
// strips EXIF metadata (preserving ICC profile).
//
// Two-stage strategy (since v0.3):
//  1. Try jpegtran -perfect (truly lossless rotation). For most JPEGs whose
//     internal storage is iMCU-aligned (real-camera output, libjpeg-encoded
//     output), this is fast and pixel-perfect.
//  2. If jpegtran -perfect fails (the source JPEG was encoded without
//     iMCU-aligned padding — e.g. some non-libjpeg encoders), fall back to a
//     decode → rotate → re-encode path. This is lossy in the JPEG re-encode
//     sense but **preserves every pixel**: no edge trimming. The original
//     ICC profile is re-injected so color management round-trips.
//
// If orientation is 1 (normal) or not present, returns a copy of the input.
// Returns an error for empty input.
func NormalizeJpegOrientation(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("modernimage: empty input data")
	}

	orientation := JpegOrientation(data)
	if orientation <= 1 {
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	}

	// Stage 1: try jpegtran -perfect (no -trim).
	out, perfErr := normalizeViaJpegtranPerfect(data, orientation)
	if perfErr == nil {
		return out, nil
	}

	// Stage 2: fall back to decode → rotate → re-encode (preserves every
	// pixel, re-injects original ICC).
	out, fbErr := normalizeViaDecodeRotateEncode(data, orientation)
	if fbErr != nil {
		return nil, fmt.Errorf(
			"modernimage: jpegtran -perfect failed (%v) and decode fallback also failed: %w",
			perfErr, fbErr,
		)
	}
	return out, nil
}

// normalizeViaJpegtranPerfect runs jpegtran in strict (lossless) mode.
// Returns an error for any non-iMCU-aligned input.
func normalizeViaJpegtranPerfect(data []byte, orientation int) ([]byte, error) {
	transformArgs, ok := orientationToJpegtranArgs(orientation)
	if !ok {
		// Should never happen for orientation 2..8, but match the original
		// behavior of returning the input verbatim.
		out := make([]byte, len(data))
		copy(out, data)
		return out, nil
	}

	tmpOut, err := createTempFile(".jpg")
	if err != nil {
		return nil, fmt.Errorf("modernimage: %w", err)
	}
	defer os.Remove(tmpOut)

	// -perfect makes jpegtran refuse non-iMCU-aligned operations instead of
	// silently dropping edge pixels (which is what -trim used to do).
	argv := []string{"jpegtran", "-copy", "icc", "-perfect"}
	argv = append(argv, transformArgs...)
	argv = append(argv, "-outfile", tmpOut)

	result, err := callTool(toolJpegtran, data, argv)
	if err != nil {
		return nil, fmt.Errorf("jpegtran -perfect orientation %d: %w", orientation, err)
	}
	return result.Data, nil
}

func orientationToJpegtranArgs(orientation int) ([]string, bool) {
	switch orientation {
	case 2:
		return []string{"-flip", "horizontal"}, true
	case 3:
		return []string{"-rotate", "180"}, true
	case 4:
		return []string{"-flip", "vertical"}, true
	case 5:
		return []string{"-transpose"}, true
	case 6:
		return []string{"-rotate", "90"}, true
	case 7:
		return []string{"-transverse"}, true
	case 8:
		return []string{"-rotate", "270"}, true
	}
	return nil, false
}

// Version returns the libmodernimage version string.
func Version() string {
	return C.GoString(C.modernimage_version())
}
