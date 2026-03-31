use thiserror::Error;

#[derive(Error, Debug)]
pub enum ModernImageError {
    #[error("empty input data")]
    EmptyInput,

    #[error("unsupported format for {op} (expected {expected}, got \"{got}\")")]
    UnsupportedFormat {
        op: String,
        expected: String,
        got: String,
    },

    #[error("failed to create context")]
    ContextCreation,

    #[error("tool exited with code {code}: {message}")]
    ToolFailed { code: i32, message: String },

    #[error("encoding produced empty output")]
    EmptyOutput,

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
}
