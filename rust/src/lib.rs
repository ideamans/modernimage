pub mod error;
pub mod ffi;

use error::ModernImageError;
use std::ffi::{CStr, CString};
use std::fs;
use std::os::raw::{c_char, c_void};

pub type Result<T> = std::result::Result<T, ModernImageError>;

pub struct EncodeResult {
    pub data: Vec<u8>,
    pub mime_type: String,
}

fn detect_format(data: &[u8]) -> &str {
    if data.len() < 4 {
        return "";
    }
    if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
        return "jpeg";
    }
    if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
        return "png";
    }
    if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 {
        return "gif";
    }
    ""
}

struct Context {
    ptr: *mut ffi::ModernImageContext,
}

impl Context {
    fn new() -> Result<Self> {
        let ptr = unsafe { ffi::modernimage_context_new() };
        if ptr.is_null() {
            return Err(ModernImageError::ContextCreation);
        }
        Ok(Context { ptr })
    }
}

impl Drop for Context {
    fn drop(&mut self) {
        if !self.ptr.is_null() {
            unsafe { ffi::modernimage_context_free(self.ptr) }
        }
    }
}

enum Tool {
    Cwebp,
    Gif2webp,
    Avifenc,
    Jpegtran,
}

fn call_tool(tool: Tool, input_data: &[u8], argv: &[&str], use_stdin: bool) -> Result<Vec<u8>> {
    let ctx = Context::new()?;

    if use_stdin {
        unsafe {
            ffi::modernimage_set_stdin(
                ctx.ptr,
                input_data.as_ptr() as *const c_void,
                input_data.len(),
            );
        }
    }

    let c_args: Vec<CString> = argv
        .iter()
        .map(|s| CString::new(*s).unwrap())
        .collect();
    let c_argv: Vec<*const c_char> = c_args.iter().map(|s| s.as_ptr()).collect();

    let rc = unsafe {
        let argc = c_argv.len() as i32;
        let argv_ptr = c_argv.as_ptr();
        match tool {
            Tool::Cwebp => ffi::modernimage_cwebp(ctx.ptr, argc, argv_ptr),
            Tool::Gif2webp => ffi::modernimage_gif2webp(ctx.ptr, argc, argv_ptr),
            Tool::Avifenc => ffi::modernimage_avifenc(ctx.ptr, argc, argv_ptr),
            Tool::Jpegtran => ffi::modernimage_jpegtran(ctx.ptr, argc, argv_ptr),
        }
    };

    if rc != 0 {
        let stderr_size = unsafe { ffi::modernimage_get_stderr_size(ctx.ptr) };
        let message = if stderr_size > 0 {
            let mut buf = vec![0u8; stderr_size];
            unsafe {
                ffi::modernimage_copy_stderr(ctx.ptr, buf.as_mut_ptr() as *mut c_char, stderr_size);
            }
            String::from_utf8_lossy(&buf).to_string()
        } else {
            String::new()
        };
        return Err(ModernImageError::ToolFailed { code: rc, message });
    }

    let out_path = argv
        .windows(2)
        .find(|w| w[0] == "-o" || w[0] == "-outfile")
        .map(|w| w[1])
        .ok_or_else(|| {
            ModernImageError::Io(std::io::Error::new(
                std::io::ErrorKind::Other,
                "no output path in argv",
            ))
        })?;

    let out_data = fs::read(out_path)?;
    if out_data.is_empty() {
        return Err(ModernImageError::EmptyOutput);
    }

    Ok(out_data)
}

/// WebP encoding functions.
pub mod webp {
    use super::*;

    /// Encode JPEG/PNG to lossy WebP. ICC profiles are preserved.
    pub fn encode_lossy(data: &[u8], quality: u32, multithread: bool) -> Result<EncodeResult> {
        if data.is_empty() {
            return Err(ModernImageError::EmptyInput);
        }
        let format = detect_format(data);
        if format != "jpeg" && format != "png" {
            return Err(ModernImageError::UnsupportedFormat {
                op: "encode_lossy".into(),
                expected: "JPEG or PNG".into(),
                got: format.into(),
            });
        }

        let tmp = tempfile::Builder::new().suffix(".webp").tempfile()?;
        let tmp_path = tmp.path().to_str().unwrap().to_string();

        let q_str = quality.to_string();
        let mut argv = vec!["cwebp", "-q", &q_str, "-metadata", "icc"];
        if multithread {
            argv.push("-mt");
        }
        argv.extend_from_slice(&["-o", &tmp_path, "--", "-"]);

        let out_data = call_tool(Tool::Cwebp, data, &argv, true)?;
        Ok(EncodeResult {
            data: out_data,
            mime_type: "image/webp".into(),
        })
    }

    /// Encode JPEG/PNG to lossless WebP. ICC profiles are preserved.
    pub fn encode_lossless(data: &[u8], multithread: bool) -> Result<EncodeResult> {
        if data.is_empty() {
            return Err(ModernImageError::EmptyInput);
        }
        let format = detect_format(data);
        if format != "jpeg" && format != "png" {
            return Err(ModernImageError::UnsupportedFormat {
                op: "encode_lossless".into(),
                expected: "JPEG or PNG".into(),
                got: format.into(),
            });
        }

        let tmp = tempfile::Builder::new().suffix(".webp").tempfile()?;
        let tmp_path = tmp.path().to_str().unwrap().to_string();

        let mut argv = vec!["cwebp", "-lossless", "-metadata", "icc"];
        if multithread {
            argv.push("-mt");
        }
        argv.extend_from_slice(&["-o", &tmp_path, "--", "-"]);

        let out_data = call_tool(Tool::Cwebp, data, &argv, true)?;
        Ok(EncodeResult {
            data: out_data,
            mime_type: "image/webp".into(),
        })
    }

    /// Encode GIF to animated WebP.
    pub fn encode_gif(data: &[u8], multithread: bool) -> Result<EncodeResult> {
        if data.is_empty() {
            return Err(ModernImageError::EmptyInput);
        }
        let format = detect_format(data);
        if format != "gif" {
            return Err(ModernImageError::UnsupportedFormat {
                op: "encode_gif".into(),
                expected: "GIF".into(),
                got: format.into(),
            });
        }

        let tmp_in = tempfile::Builder::new().suffix(".gif").tempfile()?;
        fs::write(tmp_in.path(), data)?;
        let tmp_in_path = tmp_in.path().to_str().unwrap().to_string();

        let tmp_out = tempfile::Builder::new().suffix(".webp").tempfile()?;
        let tmp_out_path = tmp_out.path().to_str().unwrap().to_string();

        let mut argv = vec!["gif2webp"];
        if multithread {
            argv.push("-mt");
        }
        argv.extend_from_slice(&[&tmp_in_path, "-o", &tmp_out_path]);

        let out_data = call_tool(Tool::Gif2webp, data, &argv, false)?;
        Ok(EncodeResult {
            data: out_data,
            mime_type: "image/webp".into(),
        })
    }
}

/// AVIF encoding functions.
pub mod avif {
    use super::*;

    struct AvifPreset {
        speed: u32,
        default_jobs: u32,
        yuv: &'static str,
        cicp: &'static str,
        depth: Option<u32>,
        auto_tile: bool,
        tile_rows: Option<u32>,
        tile_cols: Option<u32>,
    }

    const PRESET_BALANCED: AvifPreset = AvifPreset {
        speed: 6, default_jobs: 16, yuv: "444", cicp: "1/1/1",
        depth: None, auto_tile: true, tile_rows: None, tile_cols: None,
    };

    const PRESET_COMPACT: AvifPreset = AvifPreset {
        speed: 0, default_jobs: 0, yuv: "444", cicp: "1/1/1",
        depth: Some(10), auto_tile: true, tile_rows: None, tile_cols: None,
    };

    const PRESET_FAST: AvifPreset = AvifPreset {
        speed: 9, default_jobs: 16, yuv: "420", cicp: "1/13/6",
        depth: None, auto_tile: false, tile_rows: Some(6), tile_cols: Some(6),
    };

    fn encode_avif_internal(
        data: &[u8],
        quality: u32,
        jobs: u32,
        preset: &AvifPreset,
        op_name: &str,
    ) -> Result<EncodeResult> {
        if data.is_empty() {
            return Err(ModernImageError::EmptyInput);
        }
        let format = detect_format(data);
        if format != "jpeg" && format != "png" {
            return Err(ModernImageError::UnsupportedFormat {
                op: op_name.into(),
                expected: "JPEG or PNG".into(),
                got: format.into(),
            });
        }

        let tmp = tempfile::Builder::new().suffix(".avif").tempfile()?;
        let tmp_path = tmp.path().to_str().unwrap().to_string();

        let q_str = quality.to_string();
        let s_str = preset.speed.to_string();

        let mut argv = vec![
            "avifenc", "-q", &q_str, "-s", &s_str,
            "--yuv", preset.yuv, "--cicp", preset.cicp,
        ];

        let d_str;
        if let Some(depth) = preset.depth {
            d_str = depth.to_string();
            argv.extend_from_slice(&["-d", &d_str]);
        }

        let effective_jobs = if jobs > 0 {
            jobs
        } else if preset.default_jobs == 0 {
            std::thread::available_parallelism().map(|n| n.get() as u32).unwrap_or(4)
        } else {
            preset.default_jobs
        };
        let j_str = effective_jobs.to_string();
        argv.extend_from_slice(&["-j", &j_str]);

        if preset.auto_tile {
            argv.push("--autotiling");
        }
        let tr_str;
        if let Some(tr) = preset.tile_rows {
            tr_str = tr.to_string();
            argv.extend_from_slice(&["--tilerowslog2", &tr_str]);
        }
        let tc_str;
        if let Some(tc) = preset.tile_cols {
            tc_str = tc.to_string();
            argv.extend_from_slice(&["--tilecolslog2", &tc_str]);
        }

        argv.extend_from_slice(&["-a", "tune=ssimulacra2"]);
        argv.extend_from_slice(&["--input-format", format, "-o", &tmp_path, "--stdin"]);

        let out_data = call_tool(Tool::Avifenc, data, &argv, true)?;
        Ok(EncodeResult {
            data: out_data,
            mime_type: "image/avif".into(),
        })
    }

    /// Encode JPEG/PNG to AVIF with balanced speed/quality.
    /// YUV444, CICP 1/1/1 (BT.709), speed 6, autotiling, tune=ssimulacra2.
    pub fn encode_balanced(data: &[u8], quality: u32, jobs: u32) -> Result<EncodeResult> {
        encode_avif_internal(data, quality, jobs, &PRESET_BALANCED, "encode_balanced")
    }

    /// Encode JPEG/PNG to AVIF with best compression (slowest).
    /// YUV444, CICP 1/1/1 (BT.709), speed 0, 10-bit depth, autotiling, tune=ssimulacra2.
    pub fn encode_compact(data: &[u8], quality: u32, jobs: u32) -> Result<EncodeResult> {
        encode_avif_internal(data, quality, jobs, &PRESET_COMPACT, "encode_compact")
    }

    /// Encode JPEG/PNG to AVIF with fastest speed.
    /// YUV420, CICP 1/13/6 (BT.709/sRGB/BT.601), speed 9, 64x64 tiling, tune=ssimulacra2.
    pub fn encode_fast(data: &[u8], quality: u32, jobs: u32) -> Result<EncodeResult> {
        encode_avif_internal(data, quality, jobs, &PRESET_FAST, "encode_fast")
    }
}

/// JPEG orientation and normalization functions.
pub mod jpeg {
    use super::*;

    /// Returns the EXIF orientation value (1-8) from JPEG data.
    /// Returns 0 if no orientation tag is found or data is not JPEG.
    /// This is a pure read-only operation — very fast, no decompression needed.
    pub fn orientation(data: &[u8]) -> i32 {
        if data.is_empty() {
            return 0;
        }
        unsafe {
            ffi::modernimage_jpeg_orientation(data.as_ptr() as *const c_void, data.len())
        }
    }

    /// Applies lossless rotation based on EXIF orientation, then strips EXIF
    /// metadata (preserving ICC profile).
    /// If orientation is 1 (normal) or not present, returns a clone of the original data.
    pub fn normalize_orientation(data: &[u8]) -> Result<Vec<u8>> {
        if data.is_empty() {
            return Err(ModernImageError::EmptyInput);
        }

        let ori = orientation(data);
        if ori <= 1 {
            return Ok(data.to_vec());
        }

        let transform_args: &[&str] = match ori {
            2 => &["-flip", "horizontal"],
            3 => &["-rotate", "180"],
            4 => &["-flip", "vertical"],
            5 => &["-transpose"],
            6 => &["-rotate", "90"],
            7 => &["-transverse"],
            8 => &["-rotate", "270"],
            _ => return Ok(data.to_vec()),
        };

        let tmp = tempfile::Builder::new().suffix(".jpg").tempfile()?;
        let tmp_path = tmp.path().to_str().unwrap().to_string();

        let mut argv = vec!["jpegtran", "-copy", "icc", "-trim"];
        argv.extend_from_slice(transform_args);
        argv.extend_from_slice(&["-outfile", &tmp_path]);

        call_tool(Tool::Jpegtran, data, &argv, true)
    }
}

/// Get the libmodernimage version string.
pub fn version() -> String {
    unsafe {
        let v = ffi::modernimage_version();
        CStr::from_ptr(v).to_string_lossy().to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;

    fn load_test_data(name: &str) -> Vec<u8> {
        let path = Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("..")
            .join("testdata")
            .join(name);
        fs::read(&path).unwrap_or_else(|e| panic!("failed to load {}: {}", path.display(), e))
    }

    fn is_webp(data: &[u8]) -> bool {
        data.len() >= 12
            && data[0] == b'R' && data[1] == b'I' && data[2] == b'F' && data[3] == b'F'
            && data[8] == b'W' && data[9] == b'E' && data[10] == b'B' && data[11] == b'P'
    }

    fn is_avif(data: &[u8]) -> bool {
        data.len() >= 12
            && data[4] == b'f' && data[5] == b't' && data[6] == b'y' && data[7] == b'p'
    }

    const JPEG_FILES: &[&str] = &[
        "photo.jpg", "photo-like.jpg", "landscape-like.jpg",
        "medium-512x512.jpg", "edges.jpg", "gradient-radial.jpg", "small-128x128.jpg",
    ];

    const PNG_FILES: &[&str] = &[
        "logo.png", "photo-like.png", "text.png",
        "flat-color.png", "gradient-horizontal.png", "small-128x128.png",
    ];

    const GIF_FILES: &[&str] = &[
        "animation.gif", "animated-3frames.gif", "animated-small.gif",
        "static-512x512.gif", "static-alpha.gif",
    ];

    #[test]
    fn test_version() {
        let v = version();
        assert!(!v.is_empty());
    }

    #[test]
    fn test_webp_lossy_all_jpeg() {
        for name in JPEG_FILES {
            let data = load_test_data(name);
            let result = webp::encode_lossy(&data, 80, false)
                .unwrap_or_else(|e| panic!("{}: {}", name, e));
            assert!(is_webp(&result.data), "{}: not WebP", name);
        }
    }

    #[test]
    fn test_webp_lossy_all_png() {
        for name in PNG_FILES {
            let data = load_test_data(name);
            let result = webp::encode_lossy(&data, 80, true)
                .unwrap_or_else(|e| panic!("{}: {}", name, e));
            assert!(is_webp(&result.data), "{}: not WebP", name);
        }
    }

    #[test]
    fn test_webp_lossless_all() {
        for name in JPEG_FILES.iter().chain(PNG_FILES.iter()) {
            let data = load_test_data(name);
            let result = webp::encode_lossless(&data, false)
                .unwrap_or_else(|e| panic!("{}: {}", name, e));
            assert!(is_webp(&result.data), "{}: not WebP", name);
        }
    }

    #[test]
    fn test_webp_gif_all() {
        for name in GIF_FILES {
            let data = load_test_data(name);
            let result = webp::encode_gif(&data, false)
                .unwrap_or_else(|e| panic!("{}: {}", name, e));
            assert!(is_webp(&result.data), "{}: not WebP", name);
        }
    }

    #[test]
    fn test_avif_balanced_all() {
        for name in JPEG_FILES.iter().chain(PNG_FILES.iter()) {
            let data = load_test_data(name);
            let result = avif::encode_balanced(&data, 80, 0)
                .unwrap_or_else(|e| panic!("{}: {}", name, e));
            assert!(is_avif(&result.data), "{}: not AVIF", name);
        }
    }

    #[test]
    fn test_avif_compact_small() {
        for name in &["photo.jpg", "small-128x128.jpg", "logo.png", "small-128x128.png"] {
            let data = load_test_data(name);
            let result = avif::encode_compact(&data, 80, 0)
                .unwrap_or_else(|e| panic!("{}: {}", name, e));
            assert!(is_avif(&result.data), "{}: not AVIF", name);
        }
    }

    #[test]
    fn test_avif_fast_all() {
        for name in JPEG_FILES.iter().chain(PNG_FILES.iter()) {
            let data = load_test_data(name);
            let result = avif::encode_fast(&data, 80, 0)
                .unwrap_or_else(|e| panic!("{}: {}", name, e));
            assert!(is_avif(&result.data), "{}: not AVIF", name);
        }
    }

    fn inject_exif_orientation(jpeg: &[u8], orientation: u8) -> Vec<u8> {
        assert!(jpeg.len() >= 2 && jpeg[0] == 0xFF && jpeg[1] == 0xD8);
        let exif: Vec<u8> = vec![
            0xFF, 0xE1, // APP1 marker
            0x00, 0x22, // length = 34
            b'E', b'x', b'i', b'f', 0x00, 0x00,
            b'M', b'M', // big-endian
            0x00, 0x2A, // TIFF magic
            0x00, 0x00, 0x00, 0x08, // offset to IFD0
            0x00, 0x01, // 1 entry
            0x01, 0x12, // tag: Orientation
            0x00, 0x03, // type: SHORT
            0x00, 0x00, 0x00, 0x01, // count: 1
            0x00, orientation, 0x00, 0x00, // value
            0x00, 0x00, 0x00, 0x00, // next IFD: none
        ];
        let mut result = Vec::with_capacity(jpeg.len() + exif.len());
        result.extend_from_slice(&jpeg[..2]); // SOI
        result.extend_from_slice(&exif);
        result.extend_from_slice(&jpeg[2..]);
        result
    }

    #[test]
    fn test_jpeg_orientation() {
        let jpeg = load_test_data("small-128x128.jpg");
        assert_eq!(jpeg::orientation(&jpeg), 0); // no EXIF

        for ori in 1u8..=8 {
            let data = inject_exif_orientation(&jpeg, ori);
            assert_eq!(jpeg::orientation(&data), ori as i32, "orientation {}", ori);
        }

        // non-JPEG
        let png = load_test_data("small-128x128.png");
        assert_eq!(jpeg::orientation(&png), 0);
        assert_eq!(jpeg::orientation(&[]), 0);
    }

    #[test]
    fn test_normalize_jpeg_orientation() {
        let jpeg = load_test_data("small-128x128.jpg");

        // No EXIF - returns clone
        let result = jpeg::normalize_orientation(&jpeg).unwrap();
        assert_eq!(result, jpeg);

        // orientation=1 - returns clone
        let data1 = inject_exif_orientation(&jpeg, 1);
        let result1 = jpeg::normalize_orientation(&data1).unwrap();
        assert_eq!(result1, data1);

        // All orientations 2-8
        for ori in 2u8..=8 {
            let data = inject_exif_orientation(&jpeg, ori);
            let result = jpeg::normalize_orientation(&data)
                .unwrap_or_else(|e| panic!("orientation {}: {}", ori, e));
            // Result should be valid JPEG
            assert!(result.len() >= 2 && result[0] == 0xFF && result[1] == 0xD8,
                    "orientation {}: not valid JPEG", ori);
            // Result should have no EXIF orientation
            let new_ori = jpeg::orientation(&result);
            assert!(new_ori <= 1, "orientation {}: result still has orientation {}", ori, new_ori);
        }
    }

    #[test]
    fn test_normalize_then_webp() {
        let jpeg = load_test_data("small-128x128.jpg");
        let data = inject_exif_orientation(&jpeg, 6);
        let normalized = jpeg::normalize_orientation(&data).unwrap();
        let result = webp::encode_lossy(&normalized, 80, false).unwrap();
        assert!(is_webp(&result.data));
    }

    #[test]
    fn test_normalize_then_avif() {
        let jpeg = load_test_data("small-128x128.jpg");
        let data = inject_exif_orientation(&jpeg, 3);
        let normalized = jpeg::normalize_orientation(&data).unwrap();
        let result = avif::encode_fast(&normalized, 80, 0).unwrap();
        assert!(is_avif(&result.data));
    }

    #[test]
    fn test_error_empty_input() {
        assert!(webp::encode_lossy(&[], 80, false).is_err());
    }

    #[test]
    fn test_error_gif_to_lossy() {
        let data = load_test_data("animation.gif");
        assert!(webp::encode_lossy(&data, 80, false).is_err());
    }

    #[test]
    fn test_error_jpeg_to_encode_gif() {
        let data = load_test_data("photo.jpg");
        assert!(webp::encode_gif(&data, false).is_err());
    }
}
