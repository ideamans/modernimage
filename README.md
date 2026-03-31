# modernimage

High-level WebP and AVIF image encoding library powered by [libmodernimage](https://github.com/ideamans/libmodernimage).

Provides 6 use-case-oriented encoding functions with sensible defaults, available as Go, TypeScript/Node.js, and Rust bindings plus a Rust CLI.

## Use Cases

### WebP Encoding

| Function | Input | Parameters | Description |
|----------|-------|------------|-------------|
| `encode_lossy` | JPEG, PNG | quality (0-100, default 80), multithread | Lossy WebP with ICC profile preservation |
| `encode_lossless` | JPEG, PNG | multithread | Lossless WebP with ICC profile preservation |
| `encode_gif` | GIF | multithread | Animated GIF to animated WebP |

### AVIF Encoding

| Function | Input | Parameters | Description |
|----------|-------|------------|-------------|
| `encode_balanced` | JPEG, PNG | quality (0-100, default 80), jobs | Balanced speed/quality (speed=6) |
| `encode_compact` | JPEG, PNG | quality (0-100, default 80), jobs | Best compression, slowest (speed=0, 10-bit) |
| `encode_fast` | JPEG, PNG | quality (0-100, default 80), jobs | Fastest encoding (speed=9) |

All AVIF presets use `tune=ssimulacra2` for perceptual quality optimization.

### AVIF Preset Details

| Parameter | Balanced | Compact | Fast |
|-----------|----------|---------|------|
| Speed | 6 | 0 | 9 |
| YUV format | 444 | 444 | 420 |
| CICP | 1/1/1 (BT.709) | 1/1/1 (BT.709) | 1/13/6 (BT.709/sRGB/BT.601) |
| Bit depth | 8 | 10 | 8 |
| Tiling | auto | auto | manual (6x6) |
| Default threads | 16 | CPU count | 16 |

## Language Bindings

| Language | Directory | Package |
|----------|-----------|---------|
| Go | [`golang/`](golang/) | `github.com/ideamans/modernimage/golang` |
| TypeScript/Node.js | [`typescript/`](typescript/) | `modernimage` |
| Rust | [`rust/`](rust/) | `modernimage` (crate) |

See each directory's README for language-specific usage and installation.

## CLI

The Rust binding includes a CLI binary:

```bash
# Lossy WebP
modernimage webp encode-lossy -q 80 -m photo.jpg -o photo.webp

# Lossless WebP
modernimage webp encode-lossless photo.png -o photo.webp

# Animated GIF to WebP
modernimage webp encode-gif animation.gif -o animation.webp

# AVIF (balanced)
modernimage avif encode-balanced -q 80 photo.jpg -o photo.avif

# AVIF (best compression)
modernimage avif encode-compact -q 80 photo.jpg -o photo.avif

# AVIF (fastest)
modernimage avif encode-fast -q 80 -j 4 photo.jpg -o photo.avif

# Read from stdin, write to stdout
cat photo.jpg | modernimage webp encode-lossy -q 80 - > photo.webp
```

## Setup (Development)

Download the pre-built libmodernimage binaries for your platform:

```bash
make setup
```

This fetches the [libmodernimage release](https://github.com/ideamans/libmodernimage/releases) and places the static/shared libraries where each binding expects them.

Run all tests:

```bash
make test-all
```

## Architecture

```
                 +-------------------+
                 | libmodernimage    |  C FFI wrapping cwebp, gif2webp, avifenc
                 | (static/shared)   |  Thread-safe, stdin/stdout via pipes
                 +--------+----------+
                          |
          +---------------+----------------+
          |               |                |
   +------+------+ +------+------+ +-------+------+
   | Go (CGO)    | | TypeScript  | | Rust (FFI)   |
   | static link | | Koffi FFI   | | static link  |
   |             | | shared lib  | | + CLI binary  |
   +-------------+ +-------------+ +--------------+
```

Each binding builds CLI arguments programmatically and communicates with the C library via:
- **stdin injection** for cwebp and avifenc input
- **temp files** for gif2webp input (no stdin support) and all output (`-o`)
- Input format auto-detected from magic bytes

## License

MIT
