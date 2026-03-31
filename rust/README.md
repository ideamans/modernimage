# modernimage - Rust Binding & CLI

Rust binding for libmodernimage, providing WebP and AVIF image encoding. Includes a CLI binary.

## Installation

### As a Library

```toml
[dependencies]
modernimage = "0.2.0"
```

### Pre-built Library

The static library (`libmodernimage.a`) must exist under `lib/{platform}/` or be specified via `LIBMODERNIMAGE_LIB_DIR`. Run the setup script from the project root:

```bash
make setup
```

## Library Usage

### WebP Encoding

```rust
use modernimage::webp;

let jpeg = std::fs::read("photo.jpg")?;

// Lossy WebP (quality 80, multi-threaded)
let result = webp::encode_lossy(&jpeg, 80, true)?;
std::fs::write("photo.webp", &result.data)?;
// result.mime_type == "image/webp"

// Lossless WebP
let result = webp::encode_lossless(&jpeg, false)?;

// Animated GIF to WebP
let gif = std::fs::read("animation.gif")?;
let result = webp::encode_gif(&gif, true)?;
```

### AVIF Encoding

```rust
use modernimage::avif;

let jpeg = std::fs::read("photo.jpg")?;

// Balanced (speed=6, YUV444, BT.709)
let result = avif::encode_balanced(&jpeg, 80, 0)?; // quality=80, jobs=auto

// Best compression (speed=0, 10-bit, YUV444, BT.709)
let result = avif::encode_compact(&jpeg, 80, 0)?;

// Fastest (speed=9, YUV420, BT.709/sRGB/BT.601)
let result = avif::encode_fast(&jpeg, 80, 4)?; // quality=80, jobs=4
```

## API

### WebP (`modernimage::webp`)

```rust
pub fn encode_lossy(data: &[u8], quality: u32, multithread: bool) -> Result<EncodeResult>
pub fn encode_lossless(data: &[u8], multithread: bool) -> Result<EncodeResult>
pub fn encode_gif(data: &[u8], multithread: bool) -> Result<EncodeResult>
```

### AVIF (`modernimage::avif`)

```rust
pub fn encode_balanced(data: &[u8], quality: u32, jobs: u32) -> Result<EncodeResult>
pub fn encode_compact(data: &[u8], quality: u32, jobs: u32) -> Result<EncodeResult>
pub fn encode_fast(data: &[u8], quality: u32, jobs: u32) -> Result<EncodeResult>
```

### Types

```rust
pub struct EncodeResult {
    pub data: Vec<u8>,
    pub mime_type: String, // "image/webp" or "image/avif"
}
```

### Parameters

| Parameter | Range | Default | Description |
|-----------|-------|---------|-------------|
| `quality` | 0-100 | 80 | Encoding quality |
| `multithread` | bool | false | Enable WebP multi-threading |
| `jobs` | 0+ | preset default | AVIF thread count (0 = auto) |

## CLI

```bash
cargo install modernimage
```

### Commands

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

# stdin/stdout
cat photo.jpg | modernimage webp encode-lossy -q 80 - > photo.webp

# Show library version
modernimage version
```

### CLI Options

| Option | Short | Description |
|--------|-------|-------------|
| `--quality` | `-q` | Quality 0-100 (default: 80) |
| `--multithread` | `-m` | Enable multi-threading (WebP) |
| `--jobs` | `-j` | Thread count (AVIF, default: 0=auto) |
| `--output` | `-o` | Output file (default: stdout) |

## Testing

```bash
cd rust
cargo test
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `LIBMODERNIMAGE_LIB_DIR` | Custom path to `libmodernimage.a` directory |
