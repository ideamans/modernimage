use std::env;
use std::path::PathBuf;

fn main() {
    let target_os = env::var("CARGO_CFG_TARGET_OS").unwrap();
    let target_arch = env::var("CARGO_CFG_TARGET_ARCH").unwrap();

    let arch = match target_arch.as_str() {
        "x86_64" => "amd64",
        "aarch64" => "arm64",
        _ => panic!("Unsupported architecture: {}", target_arch),
    };

    let os = match target_os.as_str() {
        "macos" => "darwin",
        os => os,
    };

    let platform = format!("{}-{}", os, arch);
    let manifest_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());

    // Search for libmodernimage.a
    let mut search_paths = Vec::new();

    if let Ok(lib_dir) = env::var("LIBMODERNIMAGE_LIB_DIR") {
        search_paths.push(PathBuf::from(lib_dir));
    }

    search_paths.push(manifest_dir.join("lib").join(&platform));

    let lib_path = search_paths
        .iter()
        .find(|path| path.join("libmodernimage.a").exists())
        .cloned();

    if let Some(lib_dir) = lib_path {
        println!("cargo:rustc-link-search=native={}", lib_dir.display());
        println!("cargo:rustc-link-lib=static=modernimage");
    } else {
        println!(
            "cargo:warning=libmodernimage.a not found. Searched paths: {:?}",
            search_paths
        );
    }

    // Only C++ stdlib and threading are needed; libjpeg/libpng/giflib/zlib are bundled in .a
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
            println!("cargo:rustc-link-lib=stdc++");
            println!("cargo:rustc-link-lib=ws2_32");
        }
        _ => {}
    }

    println!("cargo:rerun-if-env-changed=LIBMODERNIMAGE_LIB_DIR");
}
