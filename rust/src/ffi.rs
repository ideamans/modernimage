use std::os::raw::{c_char, c_int, c_void};

#[repr(C)]
pub struct ModernImageContext {
    _private: [u8; 0],
}

extern "C" {
    pub fn modernimage_context_new() -> *mut ModernImageContext;
    pub fn modernimage_context_free(ctx: *mut ModernImageContext);
    pub fn modernimage_context_reset(ctx: *mut ModernImageContext);

    pub fn modernimage_set_stdin(ctx: *mut ModernImageContext, data: *const c_void, size: usize);

    pub fn modernimage_cwebp(
        ctx: *mut ModernImageContext,
        argc: c_int,
        argv: *const *const c_char,
    ) -> c_int;
    pub fn modernimage_gif2webp(
        ctx: *mut ModernImageContext,
        argc: c_int,
        argv: *const *const c_char,
    ) -> c_int;
    pub fn modernimage_avifenc(
        ctx: *mut ModernImageContext,
        argc: c_int,
        argv: *const *const c_char,
    ) -> c_int;
    pub fn modernimage_jpegtran(
        ctx: *mut ModernImageContext,
        argc: c_int,
        argv: *const *const c_char,
    ) -> c_int;

    pub fn modernimage_jpeg_orientation(data: *const c_void, size: usize) -> c_int;

    pub fn modernimage_get_exit_code(ctx: *const ModernImageContext) -> c_int;
    pub fn modernimage_get_stderr_size(ctx: *const ModernImageContext) -> usize;
    pub fn modernimage_copy_stderr(
        ctx: *const ModernImageContext,
        buf: *mut c_char,
        buf_size: usize,
    ) -> usize;

    pub fn modernimage_version() -> *const c_char;
}
