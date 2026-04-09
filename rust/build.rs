use std::env;
use std::fs;
use std::io;
use std::path::PathBuf;

const LIBMODERNIMAGE_VERSION: &str = "0.3.1";
const GITHUB_REPO: &str = "ideamans/libmodernimage";

fn main() {
    let target_os = env::var("CARGO_CFG_TARGET_OS").unwrap();
    let target_arch = env::var("CARGO_CFG_TARGET_ARCH").unwrap();

    // Go-style platform name for local search paths
    let go_arch = match target_arch.as_str() {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        _ => panic!("Unsupported architecture: {}", target_arch),
    };
    let go_os = match target_os.as_str() {
        "macos" => "darwin",
        os => os,
    };
    let go_platform = format!("{}-{}", go_os, go_arch);

    // Release archive platform name
    let release_arch = match target_arch.as_str() {
        "x86_64" => "x86_64",
        "aarch64" => {
            if target_os == "macos" {
                "arm64"
            } else {
                "aarch64"
            }
        }
        other => other,
    };
    let release_os = match target_os.as_str() {
        "macos" => "darwin",
        os => os,
    };
    let release_platform = format!("{}-{}", release_os, release_arch);

    let manifest_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());

    // On Windows we link against libmodernimage's DLL (via its import
    // library .dll.a). Statically linking the fat .a into the Rust
    // test binary caused non-deterministic 0xC00000FF crashes that
    // matched a winpthreads-vs-runtime exception-handler conflict;
    // isolating libmodernimage in a DLL sidesteps the issue. See
    // golang/modernimage.go for the same reasoning in the Go binding.
    // All other platforms continue to link statically.
    let use_dll_on_windows = target_os == "windows";
    let primary_lib_filename = if use_dll_on_windows {
        "libmodernimage.dll.a"
    } else {
        "libmodernimage.a"
    };

    // Search for the primary link target in order:
    // 1. LIBMODERNIMAGE_LIB_DIR env var
    // 2. Local lib/{platform}/ (development)
    // 3. Cache directory (auto-downloaded)
    let mut search_paths = Vec::new();

    if let Ok(lib_dir) = env::var("LIBMODERNIMAGE_LIB_DIR") {
        search_paths.push(PathBuf::from(lib_dir));
    }

    search_paths.push(manifest_dir.join("lib").join(&go_platform));

    let cache_dir = get_cache_dir().join(LIBMODERNIMAGE_VERSION).join(&release_platform);
    search_paths.push(cache_dir.clone());

    let lib_dir = search_paths
        .iter()
        .find(|path| path.join(primary_lib_filename).exists())
        .cloned();

    let lib_dir = match lib_dir {
        Some(dir) => dir,
        None => {
            // Auto-download from GitHub Releases
            eprintln!(
                "{} not found locally. Downloading v{} for {}...",
                primary_lib_filename, LIBMODERNIMAGE_VERSION, release_platform
            );
            match download_library(LIBMODERNIMAGE_VERSION, &release_platform, &cache_dir, use_dll_on_windows) {
                Ok(()) => {
                    eprintln!("Downloaded to {}", cache_dir.display());
                    cache_dir
                }
                Err(e) => {
                    panic!(
                        "Failed to download libmodernimage v{} for {}: {}\n\
                         Searched paths: {:?}\n\
                         Set LIBMODERNIMAGE_LIB_DIR to provide the library manually.",
                        LIBMODERNIMAGE_VERSION, release_platform, e, search_paths
                    );
                }
            }
        }
    };

    println!("cargo:rustc-link-search=native={}", lib_dir.display());
    if use_dll_on_windows {
        // Dynamic link: cargo emits -lmodernimage which resolves via
        // libmodernimage.dll.a (the MinGW import library), so the DLL
        // is loaded at process start. The DLL must be alongside the
        // final executable or on PATH at runtime.
        println!("cargo:rustc-link-lib=dylib=modernimage");
    } else {
        println!("cargo:rustc-link-lib=static=modernimage");
    }

    // System libraries (libjpeg/libpng/giflib/zlib are bundled in .a)
    match target_os.as_str() {
        "macos" => {
            println!("cargo:rustc-link-lib=c++");
            println!("cargo:rustc-link-lib=framework=CoreFoundation");
        }
        "linux" => {
            println!("cargo:rustc-link-lib=stdc++");
            println!("cargo:rustc-link-lib=m");
            println!("cargo:rustc-link-lib=pthread");
        }
        "windows" => {
            // When linking the DLL, all transitive deps (stdc++,
            // ws2_32, ole32, shlwapi, pthread, ...) live inside
            // libmodernimage.dll and we don't need to pull them in
            // again at the Rust link step. Only the static link
            // path needs these system libraries.
            if !use_dll_on_windows {
                // MSVC uses msvcrt (no stdc++); MinGW uses stdc++
                let target_env = env::var("CARGO_CFG_TARGET_ENV").unwrap_or_default();
                if target_env == "gnu" {
                    println!("cargo:rustc-link-lib=stdc++");
                }
                println!("cargo:rustc-link-lib=ws2_32");
                println!("cargo:rustc-link-lib=ole32");
                println!("cargo:rustc-link-lib=shlwapi");
            }
        }
        _ => {}
    }

    println!("cargo:rerun-if-env-changed=LIBMODERNIMAGE_LIB_DIR");
}

fn get_cache_dir() -> PathBuf {
    if let Ok(dir) = env::var("LIBMODERNIMAGE_CACHE_DIR") {
        return PathBuf::from(dir);
    }
    dirs::cache_dir()
        .unwrap_or_else(|| PathBuf::from(".cache"))
        .join("libmodernimage")
}

fn download_library(
    version: &str,
    release_platform: &str,
    dest_dir: &PathBuf,
    windows_dll: bool,
) -> Result<(), Box<dyn std::error::Error>> {
    let archive_name = format!("libmodernimage-{}.tar.gz", release_platform);
    let url = format!(
        "https://github.com/{}/releases/download/v{}/{}",
        GITHUB_REPO, version, archive_name
    );

    let resp = ureq::get(&url).call()?;

    if resp.status() != 200 {
        return Err(format!("HTTP {}: {}", resp.status(), url).into());
    }

    let reader = resp.into_reader();
    let gz = flate2::read::GzDecoder::new(reader);
    let mut archive = tar::Archive::new(gz);

    fs::create_dir_all(dest_dir)?;

    // Files to extract. On Windows with dynamic linking we need the
    // DLL and its import library; otherwise the static archive is
    // enough. We always extract .a as a fallback / dev convenience.
    let wanted: &[&str] = if windows_dll {
        &["libmodernimage.a", "libmodernimage.dll", "libmodernimage.dll.a"]
    } else {
        &["libmodernimage.a"]
    };

    for entry in archive.entries()? {
        let mut entry = entry?;
        let path = entry.path()?;
        let file_name = path
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("")
            .to_string();

        if wanted.contains(&file_name.as_str()) {
            let dest_path = dest_dir.join(&file_name);
            let mut file = fs::File::create(&dest_path)?;
            io::copy(&mut entry, &mut file)?;
        }
    }

    let required = if windows_dll { "libmodernimage.dll.a" } else { "libmodernimage.a" };
    if !dest_dir.join(required).exists() {
        return Err(format!("{} not found in archive", required).into());
    }

    Ok(())
}
