package modernimage

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var (
	ensureOnce  sync.Once
	ensureError error
	verbose     = os.Getenv("MODERNIMAGE_VERBOSE") != ""
)

const (
	githubRepo             = "ideamans/libmodernimage"
	baseURL                = "https://github.com/" + githubRepo + "/releases/download"
	defaultDownloadTimeout = 5 * time.Minute
)

// releasePlatform maps Go's GOOS/GOARCH to the release archive platform string.
func releasePlatform() string {
	os := runtime.GOOS
	switch runtime.GOARCH {
	case "arm64":
		if os == "darwin" {
			return "darwin-arm64"
		}
		return os + "-aarch64"
	case "amd64":
		return os + "-x86_64"
	default:
		return os + "-" + runtime.GOARCH
	}
}

// packageDir returns the directory containing this Go package's source files.
func packageDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(filename)
}

// sharedLibDir returns the directory where the static library should be placed
// for CGO to find it: shared/lib/{go-platform}/
func sharedLibDir() string {
	goPlatform := runtime.GOOS + "-" + runtime.GOARCH
	return filepath.Join(packageDir(), "shared", "lib", goPlatform)
}

// sharedIncludeDir returns shared/include/ under the package dir.
func sharedIncludeDir() string {
	return filepath.Join(packageDir(), "shared", "include")
}

// checkLibraryExists checks if the static library already exists.
func checkLibraryExists() bool {
	libPath := filepath.Join(sharedLibDir(), "libmodernimage.a")
	_, err := os.Stat(libPath)
	return err == nil
}

// downloadAndExtract downloads the release archive and extracts relevant files
// to the shared/ directories expected by CGO.
func downloadAndExtract(version string) error {
	platform := releasePlatform()
	archiveName := fmt.Sprintf("libmodernimage-%s.tar.gz", platform)
	url := fmt.Sprintf("%s/v%s/%s", baseURL, version, archiveName)

	if verbose {
		fmt.Fprintf(os.Stderr, "Downloading libmodernimage v%s for %s...\n", version, platform)
		fmt.Fprintf(os.Stderr, "URL: %s\n", url)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	libDir := sharedLibDir()
	includeDir := sharedIncludeDir()
	if err := os.MkdirAll(libDir, 0755); err != nil {
		return fmt.Errorf("failed to create lib dir: %w", err)
	}
	if err := os.MkdirAll(includeDir, 0755); err != nil {
		return fmt.Errorf("failed to create include dir: %w", err)
	}

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		baseName := filepath.Base(header.Name)
		var target string
		switch baseName {
		case "libmodernimage.a":
			target = filepath.Join(libDir, baseName)
		case "modernimage.h":
			target = filepath.Join(includeDir, baseName)
		default:
			continue // skip other files (dylib, cli-compat.json)
		}

		f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", target, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return fmt.Errorf("failed to write %s: %w", target, err)
		}
		f.Close()
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Successfully downloaded libmodernimage to %s\n", libDir)
	}
	return nil
}

// EnsureLibrary ensures the library is available, downloading if necessary.
// Safe to call multiple times; only downloads once per process.
func EnsureLibrary() error {
	ensureOnce.Do(func() {
		if checkLibraryExists() {
			return
		}
		if verbose {
			fmt.Fprintln(os.Stderr, "Pre-built library not found. Downloading from GitHub Releases...")
		}
		ensureError = downloadAndExtract(LibraryVersion)
	})
	return ensureError
}
