package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const pluginName = "censorship"

type artifactSpec struct {
	osName string
	arch   string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	versionFlag := flag.String("version", "", "release version")
	distDir := flag.String("dist", "dist", "artifact directory")
	outDir := flag.String("out", filepath.Join("dist", "release"), "output directory")
	libraryPath := flag.String("library", "", "path to one compiled plugin library")
	archivePath := flag.String("archive", "", "path to one output zip archive")
	checksumPath := flag.String("checksum", "", "path to one output checksum file")
	flag.Parse()

	if *libraryPath != "" || *archivePath != "" || *checksumPath != "" {
		if *libraryPath == "" || *archivePath == "" || *checksumPath == "" {
			return fmt.Errorf("library, archive, and checksum are required together")
		}
		version := normalizeReleaseVersion(*versionFlag)
		if err := validateReleaseVersion(version); err != nil {
			return err
		}
		library, archive, checksum, err := validateDirectPackagePaths(*libraryPath, *archivePath, *checksumPath)
		if err != nil {
			return err
		}
		if err := packageLibrary(library, archive); err != nil {
			return err
		}
		_, err = writeChecksum(checksum, archive)
		return err
	}

	version, err := resolveVersion(*versionFlag)
	if err != nil {
		return err
	}
	return packageExistingArtifacts(version, *distDir, *outDir)
}

func packageExistingArtifacts(version, distDir, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", outDir, err)
	}

	checksumLines := make([]string, 0, len(artifactSpecs()))
	for _, artifact := range artifactSpecs() {
		binaryPath := artifact.binaryPath(distDir)
		if _, err := os.Stat(binaryPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat artifact %s: %w", filepath.ToSlash(binaryPath), err)
		}
		zipName := fmt.Sprintf("%s_%s_%s_%s.zip", pluginName, version, artifact.osName, artifact.arch)
		zipPath := filepath.Join(outDir, zipName)
		if err := packageLibrary(binaryPath, zipPath); err != nil {
			return err
		}
		line, err := writeChecksum(zipPath+".sha256", zipPath)
		if err != nil {
			return err
		}
		checksumLines = append(checksumLines, line)
	}
	if len(checksumLines) == 0 {
		return fmt.Errorf("no supported artifacts found under %s", filepath.ToSlash(distDir))
	}
	return writeChecksums(filepath.Join(outDir, "checksums.txt"), checksumLines)
}

func artifactSpecs() []artifactSpec {
	return []artifactSpec{
		{osName: "linux", arch: "amd64"},
		{osName: "linux", arch: "arm64"},
		{osName: "darwin", arch: "amd64"},
		{osName: "darwin", arch: "arm64"},
		{osName: "windows", arch: "amd64"},
		{osName: "windows", arch: "arm64"},
		{osName: "freebsd", arch: "amd64"},
	}
}

func (a artifactSpec) binaryPath(distDir string) string {
	return filepath.Join(distDir, a.osName+"_"+a.arch, pluginName+libraryExtension(a.osName))
}

func libraryExtension(osName string) string {
	switch osName {
	case "windows":
		return ".dll"
	case "darwin":
		return ".dylib"
	default:
		return ".so"
	}
}

func resolveVersion(versionFlag string) (string, error) {
	if versionFlag != "" {
		version := normalizeReleaseVersion(versionFlag)
		if err := validateReleaseVersion(version); err != nil {
			return "", err
		}
		return version, nil
	}
	if rawVersion := os.Getenv("VERSION"); rawVersion != "" {
		version := normalizeReleaseVersion(rawVersion)
		if err := validateReleaseVersion(version); err != nil {
			return "", err
		}
		return version, nil
	}
	cmd := exec.Command("git", "describe", "--tags", "--exact-match")
	output, err := cmd.Output()
	if err == nil {
		version := normalizeReleaseVersion(string(output))
		if err := validateReleaseVersion(version); err != nil {
			return "", err
		}
		return version, nil
	}
	return "", fmt.Errorf("version is required: use -version, set VERSION, or run from an exact git tag")
}

func normalizeReleaseVersion(version string) string {
	version = strings.TrimSpace(version)
	return strings.TrimPrefix(version, "v")
}

func validateReleaseVersion(version string) error {
	if version == "" {
		return fmt.Errorf("release version is empty")
	}
	for i := 0; i < len(version); i++ {
		c := version[i]
		if i == 0 {
			if !isASCIIAlpha(c) && !isASCIIDigit(c) {
				return fmt.Errorf("release version %q is not a safe filename component", version)
			}
			continue
		}
		if !isASCIIAlpha(c) && !isASCIIDigit(c) && c != '.' && c != '_' && c != '+' && c != '-' {
			return fmt.Errorf("release version %q is not a safe filename component", version)
		}
	}
	return nil
}

func isASCIIAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func validateDirectPackagePaths(libraryPath, archivePath, checksumPath string) (string, string, string, error) {
	library, err := canonicalPath(libraryPath)
	if err != nil {
		return "", "", "", err
	}
	archive, err := canonicalPath(archivePath)
	if err != nil {
		return "", "", "", err
	}
	checksum, err := canonicalPath(checksumPath)
	if err != nil {
		return "", "", "", err
	}

	for _, pair := range []struct {
		firstName  string
		firstPath  string
		secondName string
		secondPath string
	}{
		{"library", library, "archive", archive},
		{"library", library, "checksum", checksum},
		{"archive", archive, "checksum", checksum},
	} {
		same, err := pathsAlias(pair.firstPath, pair.secondPath)
		if err != nil {
			return "", "", "", err
		}
		if same {
			return "", "", "", fmt.Errorf("%s and %s must refer to different files", pair.firstName, pair.secondName)
		}
	}
	return library, archive, checksum, nil
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make path absolute %s: %w", filepath.ToSlash(path), err)
	}
	return filepath.Clean(absolute), nil
}

func pathsAlias(firstPath, secondPath string) (bool, error) {
	if firstPath == secondPath || runtime.GOOS == "windows" && strings.EqualFold(firstPath, secondPath) {
		return true, nil
	}
	firstInfo, firstSuffix, err := existingPathAncestor(firstPath)
	if err != nil {
		return false, err
	}
	secondInfo, secondSuffix, err := existingPathAncestor(secondPath)
	if err != nil {
		return false, err
	}
	if !os.SameFile(firstInfo, secondInfo) || len(firstSuffix) != len(secondSuffix) {
		return false, nil
	}
	for i := range firstSuffix {
		if firstSuffix[i] != secondSuffix[i] && (runtime.GOOS != "windows" || !strings.EqualFold(firstSuffix[i], secondSuffix[i])) {
			return false, nil
		}
	}
	return true, nil
}

func existingPathAncestor(path string) (os.FileInfo, []string, error) {
	var suffix []string
	for {
		info, err := os.Stat(path)
		if err == nil {
			return info, suffix, nil
		}
		if !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("stat path %s: %w", filepath.ToSlash(path), err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil, nil, fmt.Errorf("find existing ancestor for %s", filepath.ToSlash(path))
		}
		suffix = append([]string{filepath.Base(path)}, suffix...)
		path = parent
	}
}

func packageLibrary(libraryPath, archivePath string) error {
	library, err := os.Open(libraryPath)
	if err != nil {
		return fmt.Errorf("open library %s: %w", filepath.ToSlash(libraryPath), err)
	}
	defer library.Close()

	info, err := library.Stat()
	if err != nil {
		return fmt.Errorf("stat library %s: %w", filepath.ToSlash(libraryPath), err)
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}
	archive, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create archive %s: %w", filepath.ToSlash(archivePath), err)
	}
	archiveClosed := false
	defer func() {
		if !archiveClosed {
			_ = archive.Close()
		}
	}()

	writer := zip.NewWriter(archive)
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("create zip header: %w", err)
	}
	header.Name = filepath.Base(libraryPath)
	header.Method = zip.Deflate
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", header.Name, err)
	}
	if _, err := io.Copy(entry, library); err != nil {
		return fmt.Errorf("write zip entry %s: %w", header.Name, err)
	}
	if err := addOptionalFile(writer, "LICENSE"); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("close archive %s: %w", filepath.ToSlash(archivePath), err)
	}
	archiveClosed = true
	return nil
}

func addOptionalFile(writer *zip.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open optional file %s: %w", filepath.ToSlash(path), err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat optional file %s: %w", filepath.ToSlash(path), err)
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("create optional zip header %s: %w", path, err)
	}
	header.Name = filepath.Base(path)
	header.Method = zip.Deflate
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create optional zip entry %s: %w", header.Name, err)
	}
	if _, err := io.Copy(entry, file); err != nil {
		return fmt.Errorf("write optional zip entry %s: %w", header.Name, err)
	}
	return nil
}

func writeChecksum(checksumPath, archivePath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(checksumPath), 0o755); err != nil {
		return "", fmt.Errorf("create checksum directory: %w", err)
	}
	checksum, err := sha256File(archivePath)
	if err != nil {
		return "", err
	}
	line := fmt.Sprintf("%s  %s\n", checksum, filepath.Base(archivePath))
	if err := os.WriteFile(checksumPath, []byte(line), 0o644); err != nil {
		return "", fmt.Errorf("write checksum %s: %w", filepath.ToSlash(checksumPath), err)
	}
	return line, nil
}

func writeChecksums(path string, checksumLines []string) error {
	if err := os.WriteFile(path, []byte(strings.Join(checksumLines, "")), 0o644); err != nil {
		return fmt.Errorf("write checksums %s: %w", path, err)
	}
	return nil
}

var sha256File = sha256FileImpl

func sha256FileImpl(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open zip for checksum %s: %w", path, err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash zip %s: %w", path, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
