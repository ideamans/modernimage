/*
 * libmodernimage - Thread-safe FFI bridge for cwebp, gif2webp, avifenc
 *
 * Copyright 2024 ideaman's Inc.
 * SPDX-License-Identifier: MIT
 */

#ifndef MODERNIMAGE_H_
#define MODERNIMAGE_H_

#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* Opaque context handle */
typedef struct modernimage_context modernimage_context_t;

/*
 * Lifecycle
 */

/* Create a new context. Returns NULL on allocation failure. */
modernimage_context_t* modernimage_context_new(void);

/* Free a context and all associated resources. */
void modernimage_context_free(modernimage_context_t* ctx);

/* Reset a context for reuse (clears captured output, resets exit code). */
void modernimage_context_reset(modernimage_context_t* ctx);

/*
 * Stdin injection (optional, call before execution)
 *
 * Set in-memory data that the tool will read from stdin.
 * The data is NOT copied — the caller must keep it alive until execution completes.
 *
 * For cwebp:   use "-" as the input filename
 * For avifenc: use "--stdin" flag (+ "--input-format jpeg/png")
 * For gif2webp: stdin is not supported by the underlying tool
 *
 * Pass NULL/0 to clear (no stdin injection).
 */
void modernimage_set_stdin(modernimage_context_t* ctx,
                           const void* data, size_t size);

/*
 * Tool execution (thread-safe)
 *
 * Each function executes the equivalent CLI tool with the given arguments.
 * argv[0] should be the tool name (e.g. "cwebp") for compatibility.
 * Returns the tool's exit code (0 = success).
 *
 * Multiple contexts can run different tools concurrently.
 * Same-tool calls are serialized by an internal mutex.
 */

int modernimage_cwebp(modernimage_context_t* ctx, int argc, const char* argv[]);
int modernimage_gif2webp(modernimage_context_t* ctx, int argc, const char* argv[]);
int modernimage_avifenc(modernimage_context_t* ctx, int argc, const char* argv[]);

/*
 * Output access (call after execution)
 */

/* Get the size of captured stdout/stderr data in bytes. */
size_t modernimage_get_stdout_size(const modernimage_context_t* ctx);
size_t modernimage_get_stderr_size(const modernimage_context_t* ctx);

/*
 * Copy captured stdout/stderr into caller-owned buffer.
 * Copies at most buf_size bytes (truncates if insufficient).
 * Returns the number of bytes actually copied.
 */
size_t modernimage_copy_stdout(const modernimage_context_t* ctx,
                               char* buf, size_t buf_size);
size_t modernimage_copy_stderr(const modernimage_context_t* ctx,
                               char* buf, size_t buf_size);

/* Get the exit code from the last execution. */
int modernimage_get_exit_code(const modernimage_context_t* ctx);

/*
 * Version info
 */
const char* modernimage_version(void);

#ifdef __cplusplus
}
#endif

#endif /* MODERNIMAGE_H_ */
