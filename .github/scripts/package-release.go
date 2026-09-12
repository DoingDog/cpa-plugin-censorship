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
	"strings"
	"time"
)

const pluginName = "censorship"

var (
	zipModifiedTime  = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	renameOutputFile = os.Rename
)

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
	packages := make([]struct {
		library  string
		archive  string
		checksum string
	}, 0, len(artifactSpecs()))
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
		library, archive, checksum, err := validateDirectPackagePaths(binaryPath, zipPath, zipPath+".sha256")
		if err != nil {
			return err
		}
		packages = append(packages, struct {
			library  string
			archive  string
			checksum string
		}{library, archive, checksum})
	}

	if len(packages) == 0 {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("create output dir %s: %w", outDir, err)
		}
		return fmt.Errorf("no supported artifacts found under %s", filepath.ToSlash(distDir))
	}

	checksumsPath := filepath.Join(outDir, "checksums.txt")
	if err := rejectOutputSymlink(checksumsPath, "checksums.txt"); err != nil {
		return err
	}
	checksums, err := canonicalPath(checksumsPath)
	if err != nil {
		return err
	}
	outputs := make([]struct {
		name string
		path string
	}, 0, len(packages)*2+1)
	for _, artifact := range packages {
		outputs = append(outputs,
			struct {
				name string
				path string
			}{"archive", artifact.archive},
			struct {
				name string
				path string
			}{"checksum", artifact.checksum},
		)
	}
	outputs = append(outputs, struct {
		name string
		path string
	}{"checksums.txt", checksums})
	for index, output := range outputs {
		for _, artifact := range packages {
			same, err := pathsAlias(artifact.library, output.path)
			if err != nil {
				return err
			}
			if same {
				return fmt.Errorf("library and %s must refer to different files", output.name)
			}
		}
		for _, previous := range outputs[:index] {
			same, err := pathsAlias(previous.path, output.path)
			if err != nil {
				return err
			}
			if same {
				return fmt.Errorf("%s and %s must refer to different files", previous.name, output.name)
			}
		}
	}
	if _, err := os.Lstat("LICENSE"); err == nil {
		if _, err := os.Stat("LICENSE"); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("LICENSE must not be a dangling symlink")
			}
			return fmt.Errorf("inspect LICENSE: %w", err)
		}
		license, err := canonicalPath("LICENSE")
		if err != nil {
			return err
		}
		same, err := pathsAlias(license, checksums)
		if err != nil {
			return err
		}
		if same {
			return fmt.Errorf("LICENSE and checksums.txt must refer to different files")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect LICENSE: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", outDir, err)
	}
	checksumLines := make([]string, 0, len(packages))
	for _, artifact := range packages {
		if err := packageLibrary(artifact.library, artifact.archive); err != nil {
			return err
		}
		line, err := writeChecksum(artifact.checksum, artifact.archive)
		if err != nil {
			return err
		}
		checksumLines = append(checksumLines, line)
	}
	return writeChecksums(checksums, checksumLines)
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
		version := normalizeReleaseVersion(strings.TrimRight(string(output), "\r\n"))
		if err := validateReleaseVersion(version); err != nil {
			return "", err
		}
		return version, nil
	}
	return "", fmt.Errorf("version is required: use -version, set VERSION, or run from an exact git tag")
}

func normalizeReleaseVersion(version string) string {
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
	if err := rejectOutputSymlink(archivePath, "archive"); err != nil {
		return "", "", "", err
	}
	if err := rejectOutputSymlink(checksumPath, "checksum"); err != nil {
		return "", "", "", err
	}

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

	if _, err := os.Lstat("LICENSE"); err == nil {
		if _, err := os.Stat("LICENSE"); err != nil {
			if os.IsNotExist(err) {
				return "", "", "", fmt.Errorf("LICENSE must not be a dangling symlink")
			}
			return "", "", "", fmt.Errorf("inspect LICENSE: %w", err)
		}
		license, err := canonicalPath("LICENSE")
		if err != nil {
			return "", "", "", err
		}
		for _, output := range []struct {
			name string
			path string
		}{
			{"archive", archive},
			{"checksum", checksum},
		} {
			same, err := pathsAlias(license, output.path)
			if err != nil {
				return "", "", "", err
			}
			if same {
				return "", "", "", fmt.Errorf("LICENSE and %s must refer to different files", output.name)
			}
		}
	} else if !os.IsNotExist(err) {
		return "", "", "", fmt.Errorf("inspect LICENSE: %w", err)
	}

	return library, archive, checksum, nil
}

func rejectOutputSymlink(path, name string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %s %s: %w", name, filepath.ToSlash(path), err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", name, filepath.ToSlash(path))
	}
	return nil
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make path absolute %s: %w", filepath.ToSlash(path), err)
	}
	return filepath.Clean(absolute), nil
}

func pathsAlias(firstPath, secondPath string) (bool, error) {
	if firstPath == secondPath {
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
		if !asciiEqualFold(firstSuffix[i], secondSuffix[i]) {
			return false, nil
		}
	}
	return true, nil
}

func asciiEqualFold(first, second string) bool {
	if len(first) != len(second) {
		return false
	}
	for i := 0; i < len(first); i++ {
		if first[i] == second[i] {
			continue
		}
		if first[i] >= 'A' && first[i] <= 'Z' {
			if first[i]+'a'-'A' == second[i] {
				continue
			}
		} else if first[i] >= 'a' && first[i] <= 'z' && first[i]-'a'+'A' == second[i] {
			continue
		}
		return false
	}
	return true
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

func reserveOutputBackup(path string) (string, error) {
	backup, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".backup-*")
	if err != nil {
		return "", err
	}
	name := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func replaceOutputFile(tempPath, destination string) error {
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return renameOutputFile(tempPath, destination)
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("release output %q is not a regular file", destination)
	}
	backup, err := reserveOutputBackup(destination)
	if err != nil {
		return fmt.Errorf("reserve output backup %q: %w", destination, err)
	}
	if err := renameOutputFile(destination, backup); err != nil {
		return fmt.Errorf("stage existing output %q: %w", destination, err)
	}
	if err := renameOutputFile(tempPath, destination); err != nil {
		if rollbackErr := renameOutputFile(backup, destination); rollbackErr != nil {
			return fmt.Errorf("install output %q: %w; restore failed: %v; old output retained at %q", destination, err, rollbackErr, backup)
		}
		return fmt.Errorf("install output %q: %w", destination, err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("remove replaced output backup %q: %w", backup, err)
	}
	return nil
}

func writeOutputFile(path string, perm os.FileMode, write func(*os.File) error) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if temporary != nil {
			_ = temporary.Close()
		}
		if temporaryPath != "" {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(perm); err != nil {
		return err
	}
	if err := write(temporary); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	temporary = nil
	if err := replaceOutputFile(temporaryPath, path); err != nil {
		return err
	}
	temporaryPath = ""
	return nil
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
	if err := writeOutputFile(archivePath, 0o644, func(archive *os.File) error {
		writer := zip.NewWriter(archive)
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("create zip header: %w", err)
		}
		header.Modified = zipModifiedTime
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
		return nil
	}); err != nil {
		return fmt.Errorf("write archive %s: %w", filepath.ToSlash(archivePath), err)
	}
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
	header.Modified = zipModifiedTime
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
	checksum, err := sha256File(archivePath)
	if err != nil {
		return "", err
	}
	line := fmt.Sprintf("%s  %s\n", checksum, filepath.Base(archivePath))
	if err := writeOutputFile(checksumPath, 0o644, func(file *os.File) error {
		_, err := io.WriteString(file, line)
		return err
	}); err != nil {
		return "", fmt.Errorf("write checksum %s: %w", filepath.ToSlash(checksumPath), err)
	}
	return line, nil
}

func writeChecksums(path string, checksumLines []string) error {
	contents := strings.Join(checksumLines, "")
	if err := writeOutputFile(path, 0o644, func(file *os.File) error {
		_, err := io.WriteString(file, contents)
		return err
	}); err != nil {
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
