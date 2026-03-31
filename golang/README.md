# modernimage - Go Binding

Go (CGO) binding for libmodernimage, providing WebP and AVIF image encoding.

## Installation

```bash
go get github.com/ideamans/modernimage/golang
```

### Pre-built Library

The pre-built `libmodernimage.a` must exist under `shared/lib/{platform}/` before building. Run the setup script from the project root:

```bash
make setup
```

Or use the programmatic download:

```go
import modernimage "github.com/ideamans/modernimage/golang"

if err := modernimage.EnsureLibrary(); err != nil {
    log.Fatal(err)
}
```

## Usage

### WebP Encoding

```go
package main

import (
    "os"
    modernimage "github.com/ideamans/modernimage/golang"
)

func main() {
    data, _ := os.ReadFile("photo.jpg")

    // Lossy WebP (quality 80, multi-threaded)
    result, err := modernimage.Webp.EncodeLossy(data, 80, true)
    // result.Data = WebP bytes, result.MimeType = "image/webp"

    // Lossless WebP
    result, err = modernimage.Webp.EncodeLossless(data, false)

    // Animated GIF to WebP
    gifData, _ := os.ReadFile("animation.gif")
    result, err = modernimage.Webp.EncodeGif(gifData, true)
}
```

### AVIF Encoding

```go
data, _ := os.ReadFile("photo.jpg")

// Balanced (speed=6, YUV444, BT.709)
result, err := modernimage.Avif.EncodeBalanced(data, 80, 0) // quality=80, jobs=auto

// Best compression (speed=0, 10-bit, YUV444, BT.709)
result, err = modernimage.Avif.EncodeCompact(data, 80, 0)

// Fastest (speed=9, YUV420, BT.709/sRGB/BT.601)
result, err = modernimage.Avif.EncodeFast(data, 80, 4) // quality=80, jobs=4
```

## API

### WebP (`modernimage.Webp`)

```go
func (WebpEncoder) EncodeLossy(data []byte, quality int, multithread bool) (*EncodeResult, error)
func (WebpEncoder) EncodeLossless(data []byte, multithread bool) (*EncodeResult, error)
func (WebpEncoder) EncodeGif(data []byte, multithread bool) (*EncodeResult, error)
```

### AVIF (`modernimage.Avif`)

```go
func (AvifEncoder) EncodeBalanced(data []byte, quality int, jobs int) (*EncodeResult, error)
func (AvifEncoder) EncodeCompact(data []byte, quality int, jobs int) (*EncodeResult, error)
func (AvifEncoder) EncodeFast(data []byte, quality int, jobs int) (*EncodeResult, error)
```

### Types

```go
type EncodeResult struct {
    Data     []byte
    MimeType string // "image/webp" or "image/avif"
}
```

### Parameters

| Parameter | Range | Default | Description |
|-----------|-------|---------|-------------|
| `quality` | 0-100 | 80 | Encoding quality |
| `multithread` | bool | false | Enable WebP multi-threading |
| `jobs` | 0+ | preset default | AVIF thread count (0 = auto) |

### Input Format

Input format is auto-detected from magic bytes:
- JPEG (`FF D8 FF`) - accepted by lossy, lossless, balanced, compact, fast
- PNG (`89 50 4E 47`) - accepted by lossy, lossless, balanced, compact, fast
- GIF (`47 49 46`) - accepted by encode_gif only

## Testing

```bash
cd golang
go test -v -timeout 120s
```
