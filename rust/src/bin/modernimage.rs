use clap::{Parser, Subcommand};
use modernimage::{avif, version, webp};
use std::fs;
use std::io::{self, Read, Write};
use std::process;

#[derive(Parser)]
#[command(
    name = "modernimage",
    version = option_env!("MODERNIMAGE_VERSION").unwrap_or(env!("CARGO_PKG_VERSION")),
    about = "WebP and AVIF image encoding"
)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// WebP encoding commands
    Webp {
        #[command(subcommand)]
        command: WebpCommands,
    },
    /// AVIF encoding commands
    Avif {
        #[command(subcommand)]
        command: AvifCommands,
    },
    /// Show library version
    Version,
}

#[derive(Subcommand)]
enum WebpCommands {
    /// Encode to lossy WebP
    EncodeLossy {
        /// Input file (use - for stdin)
        input: String,
        /// Quality 0-100
        #[arg(short, long, default_value = "80")]
        quality: u32,
        /// Enable multi-threaded encoding
        #[arg(short, long)]
        multithread: bool,
        /// Output file (default: stdout)
        #[arg(short, long)]
        output: Option<String>,
    },
    /// Encode to lossless WebP
    EncodeLossless {
        /// Input file (use - for stdin)
        input: String,
        /// Enable multi-threaded encoding
        #[arg(short, long)]
        multithread: bool,
        /// Output file (default: stdout)
        #[arg(short, long)]
        output: Option<String>,
    },
    /// Encode GIF to animated WebP
    EncodeGif {
        /// Input file (use - for stdin)
        input: String,
        /// Enable multi-threaded encoding
        #[arg(short, long)]
        multithread: bool,
        /// Output file (default: stdout)
        #[arg(short, long)]
        output: Option<String>,
    },
}

#[derive(Subcommand)]
enum AvifCommands {
    /// Encode to AVIF (balanced speed/quality)
    EncodeBalanced {
        /// Input file (use - for stdin)
        input: String,
        /// Quality 0-100
        #[arg(short, long, default_value = "80")]
        quality: u32,
        /// Thread count (0 = auto)
        #[arg(short, long, default_value = "0")]
        jobs: u32,
        /// Output file (default: stdout)
        #[arg(short, long)]
        output: Option<String>,
    },
    /// Encode to AVIF (best compression, slowest)
    EncodeCompact {
        /// Input file (use - for stdin)
        input: String,
        /// Quality 0-100
        #[arg(short, long, default_value = "80")]
        quality: u32,
        /// Thread count (0 = auto)
        #[arg(short, long, default_value = "0")]
        jobs: u32,
        /// Output file (default: stdout)
        #[arg(short, long)]
        output: Option<String>,
    },
    /// Encode to AVIF (fastest)
    EncodeFast {
        /// Input file (use - for stdin)
        input: String,
        /// Quality 0-100
        #[arg(short, long, default_value = "80")]
        quality: u32,
        /// Thread count (0 = auto)
        #[arg(short, long, default_value = "0")]
        jobs: u32,
        /// Output file (default: stdout)
        #[arg(short, long)]
        output: Option<String>,
    },
}

fn read_input(path: &str) -> Vec<u8> {
    if path == "-" {
        let mut buf = Vec::new();
        io::stdin().read_to_end(&mut buf).unwrap_or_else(|e| {
            eprintln!("Error reading stdin: {}", e);
            process::exit(1);
        });
        buf
    } else {
        fs::read(path).unwrap_or_else(|e| {
            eprintln!("Error reading {}: {}", path, e);
            process::exit(1);
        })
    }
}

fn write_output(data: &[u8], output: &Option<String>) {
    let result = match output {
        Some(path) => fs::write(path, data),
        None => io::stdout().write_all(data),
    };
    if let Err(e) = result {
        eprintln!("Error writing output: {}", e);
        process::exit(1);
    }
}

fn run_encode<F>(input: &str, output: &Option<String>, encode_fn: F)
where
    F: FnOnce(&[u8]) -> Result<modernimage::EncodeResult, modernimage::error::ModernImageError>,
{
    let data = read_input(input);
    match encode_fn(&data) {
        Ok(r) => write_output(&r.data, output),
        Err(e) => {
            eprintln!("Error: {}", e);
            process::exit(1);
        }
    }
}

fn main() {
    let cli = Cli::parse();

    match cli.command {
        Commands::Webp { command } => match command {
            WebpCommands::EncodeLossy {
                input,
                quality,
                multithread,
                output,
            } => run_encode(&input, &output, |d| webp::encode_lossy(d, quality, multithread)),

            WebpCommands::EncodeLossless {
                input,
                multithread,
                output,
            } => run_encode(&input, &output, |d| webp::encode_lossless(d, multithread)),

            WebpCommands::EncodeGif {
                input,
                multithread,
                output,
            } => run_encode(&input, &output, |d| webp::encode_gif(d, multithread)),
        },

        Commands::Avif { command } => match command {
            AvifCommands::EncodeBalanced {
                input,
                quality,
                jobs,
                output,
            } => run_encode(&input, &output, |d| avif::encode_balanced(d, quality, jobs)),

            AvifCommands::EncodeCompact {
                input,
                quality,
                jobs,
                output,
            } => run_encode(&input, &output, |d| avif::encode_compact(d, quality, jobs)),

            AvifCommands::EncodeFast {
                input,
                quality,
                jobs,
                output,
            } => run_encode(&input, &output, |d| avif::encode_fast(d, quality, jobs)),
        },

        Commands::Version => {
            println!("modernimage CLI");
            println!("libmodernimage version: {}", version());
        }
    }
}
