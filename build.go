//go:build ignore

// build.go is a portable build helper that replicates Makefile targets.
// Usage: go run build.go <target>
//
// Targets: build-linux, build-mac, build-windows, build-all, release, clean
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	buildDir = "build"
	relDir   = "release"
	pkgNousd = "./cmd/nousd"
	pkgCLI   = "./cmd/nous-cli"
)

var goflags = []string{"-trimpath"}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run build.go <target>")
		fmt.Println("Targets: build-linux, build-mac, build-windows, build-all, release, clean")
		os.Exit(1)
	}
	target := os.Args[1]
	switch target {
	case "build-linux":
		buildPlatform("linux", "amd64", "")
	case "build-mac":
		buildPlatform("darwin", "arm64", "")
	case "build-windows":
		buildPlatform("windows", "amd64", ".exe")
	case "build-all":
		buildPlatform("linux", "amd64", "")
		buildPlatform("darwin", "arm64", "")
		buildPlatform("windows", "amd64", ".exe")
	case "release":
		buildPlatform("linux", "amd64", "")
		buildPlatform("darwin", "arm64", "")
		buildPlatform("windows", "amd64", ".exe")
		packageRelease()
	case "clean":
		os.RemoveAll(buildDir)
		os.RemoveAll(relDir)
		fmt.Println("cleaned build/ and release/")
	default:
		fmt.Fprintf(os.Stderr, "unknown target: %s\n", target)
		os.Exit(1)
	}
}

func buildPlatform(goos, goarch, ext string) {
	dir := filepath.Join(buildDir, goos)
	os.MkdirAll(dir, 0o755)

	for _, bin := range []struct{ name, pkg string }{
		{"nousd" + ext, pkgNousd},
		{"nous-cli" + ext, pkgCLI},
	} {
		out := filepath.Join(dir, bin.name)
		args := append([]string{"build"}, goflags...)
		args = append(args, "-ldflags", "-s -w", "-o", out, bin.pkg)
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(),
			"GOOS="+goos,
			"GOARCH="+goarch,
			"CGO_ENABLED=0",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("  %s/%s %s → %s\n", goos, goarch, bin.name, out)
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Printf("  ✓ %s/%s\n", goos, goarch)
}

func packageRelease() {
	os.MkdirAll(relDir, 0o755)

	// Copy README.txt into each build dir.
	readme, err := os.ReadFile("release-readme.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		readme = []byte("NOUS Cryptocurrency\n")
	}
	for _, d := range []string{"linux", "darwin", "windows"} {
		os.WriteFile(filepath.Join(buildDir, d, "README.txt"), readme, 0o644)
	}

	// Linux tar.gz
	tarGz(filepath.Join(relDir, "nous-linux-amd64.tar.gz"),
		filepath.Join(buildDir, "linux"),
		[]string{"nousd", "nous-cli", "README.txt"})

	// macOS tar.gz
	tarGz(filepath.Join(relDir, "nous-darwin-arm64.tar.gz"),
		filepath.Join(buildDir, "darwin"),
		[]string{"nousd", "nous-cli", "README.txt"})

	// Windows zip
	zipFiles(filepath.Join(relDir, "nous-windows-amd64.zip"),
		filepath.Join(buildDir, "windows"),
		[]string{"nousd.exe", "nous-cli.exe", "README.txt"})

	fmt.Println("\nRelease archives:")
	entries, _ := os.ReadDir(relDir)
	for _, e := range entries {
		info, _ := e.Info()
		size := float64(info.Size()) / 1024 / 1024
		fmt.Printf("  %-40s %.1f MB\n", e.Name(), size)
	}

	_ = runtime.GOOS // suppress unused import
	_ = strings.TrimSpace("") // suppress unused import
}

func tarGz(outPath, baseDir string, files []string) {
	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create %s: %v\n", outPath, err)
		os.Exit(1)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, name := range files {
		path := filepath.Join(baseDir, name)
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %s: %v\n", path, err)
			os.Exit(1)
		}
		hdr, _ := tar.FileInfoHeader(info, "")
		hdr.Name = name
		if name != "README.txt" {
			hdr.Mode = 0o755
		}
		tw.WriteHeader(hdr)
		data, _ := os.ReadFile(path)
		tw.Write(data)
	}
	fmt.Printf("  ✓ %s\n", outPath)
}

func zipFiles(outPath, baseDir string, files []string) {
	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create %s: %v\n", outPath, err)
		os.Exit(1)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for _, name := range files {
		path := filepath.Join(baseDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
			os.Exit(1)
		}
		w, _ := zw.Create(name)
		io.Copy(w, strings.NewReader(string(data)))
	}
	fmt.Printf("  ✓ %s\n", outPath)
}
