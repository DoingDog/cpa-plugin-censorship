package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestArtifactSpecs(t *testing.T) {
	got := artifactSpecs()
	want := []artifactSpec{
		{osName: "linux", arch: "amd64"},
		{osName: "linux", arch: "arm64"},
		{osName: "darwin", arch: "amd64"},
		{osName: "darwin", arch: "arm64"},
		{osName: "windows", arch: "amd64"},
		{osName: "windows", arch: "arm64"},
		{osName: "freebsd", arch: "amd64"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifactSpecs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeReleaseVersionRemovesOneASCIILeadingV(t *testing.T) {
	for input, want := range map[string]string{"v1.2.3": "1.2.3", "1.2.3": "1.2.3", "vv1": "v1", "V1": "V1"} {
		if got := normalizeReleaseVersion(input); got != want {
			t.Errorf("normalizeReleaseVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidateReleaseVersion(t *testing.T) {
	valid := []string{"1.2.3", "0.1.0-rc.1+build", "0.0.0-dev", "v1"}
	for _, version := range valid {
		if err := validateReleaseVersion(normalizeReleaseVersion(version)); err != nil {
			t.Errorf("validateReleaseVersion(%q) = %v", version, err)
		}
	}
	invalid := []string{"", ".", "..", "foo/bar", `foo\bar`, "foo bar", "foo\x00bar", "foo:bar", "foo*bar", "foo?bar", "foo<bar", "foo>bar", "foo|bar"}
	for _, version := range invalid {
		if err := validateReleaseVersion(normalizeReleaseVersion(version)); err == nil {
			t.Errorf("validateReleaseVersion(%q) = nil", version)
		}
	}
}

func TestPackageLibraryAndChecksumContract(t *testing.T) {
	tmp := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.WriteFile("LICENSE", []byte("fixture license\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(tmp, "censorship.so")
	if err := os.WriteFile(library, []byte("fixture library"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(tmp, "censorship_1.2.3_linux_amd64.zip")
	checksum := archive + ".sha256"
	if err := packageLibrary(library, archive); err != nil {
		t.Fatal(err)
	}
	if _, err := writeChecksum(checksum, archive); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 2 || zr.File[0].Name != "censorship.so" || zr.File[1].Name != "LICENSE" {
		t.Fatalf("zip entries = %#v", zipNames(zr.File))
	}
	if zr.File[0].Mode().Perm() != 0o755 || strings.Contains(zr.File[0].Name, "/") {
		t.Fatalf("library header = name %q mode %v", zr.File[0].Name, zr.File[0].Mode())
	}
	line, err := os.ReadFile(checksum)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}  censorship_1\.2\.3_linux_amd64\.zip\n$`).Match(line) {
		t.Fatalf("checksum = %q", line)
	}
}

func TestDirectOutputsReplaceUnknownHardLinksWithoutChangingPeer(t *testing.T) {
	for _, output := range []string{"archive", "checksum"} {
		t.Run(output, func(t *testing.T) {
			dir := t.TempDir()
			old, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(old) })
			if err := os.WriteFile("LICENSE", []byte("license\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			library := filepath.Join(dir, "censorship.so")
			archive := filepath.Join(dir, "censorship.zip")
			checksum := archive + ".sha256"
			if err := os.WriteFile(library, []byte("library"), 0o755); err != nil {
				t.Fatal(err)
			}
			if output == "checksum" {
				if err := packageLibrary(library, archive); err != nil {
					t.Fatal(err)
				}
			}
			destination := archive
			if output == "checksum" {
				destination = checksum
			}
			sentinel := filepath.Join(dir, "sentinel")
			original := []byte("unrelated hard-link peer")
			if err := os.WriteFile(sentinel, original, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(sentinel, destination); err != nil {
				t.Fatal(err)
			}
			if output == "archive" {
				err = packageLibrary(library, archive)
			} else {
				_, err = writeChecksum(checksum, archive)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(sentinel)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, original) {
				t.Fatalf("unknown peer = %q, want %q", got, original)
			}
			destinationInfo, err := os.Stat(destination)
			if err != nil {
				t.Fatal(err)
			}
			sentinelInfo, err := os.Stat(sentinel)
			if err != nil {
				t.Fatal(err)
			}
			if os.SameFile(destinationInfo, sentinelInfo) {
				t.Fatal("output still aliases unknown peer")
			}
		})
	}
}

func TestAggregateOutputReplacesUnknownHardLink(t *testing.T) {
	dir := t.TempDir()
	dist := filepath.Join(dir, "dist")
	out := filepath.Join(dir, "out")
	library := filepath.Join(dist, "linux_amd64", "censorship.so")
	if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(library, []byte("library"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "sentinel")
	original := []byte("aggregate peer")
	if err := os.WriteFile(sentinel, original, 0o644); err != nil {
		t.Fatal(err)
	}
	aggregate := filepath.Join(out, "checksums.txt")
	if err := os.Link(sentinel, aggregate); err != nil {
		t.Fatal(err)
	}
	if err := packageExistingArtifacts("1.2.3", dist, out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("unknown aggregate peer = %q, want %q", got, original)
	}
	aggregateInfo, err := os.Stat(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	sentinelInfo, err := os.Stat(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(aggregateInfo, sentinelInfo) {
		t.Fatal("aggregate still aliases unknown peer")
	}
	contents, err := os.ReadFile(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}  censorship_1\.2\.3_linux_amd64\.zip\n$`).Match(contents) {
		t.Fatalf("aggregate = %q", contents)
	}
}

func TestPackageExistingArtifactsRemovesMissingPlatformOutputs(t *testing.T) {
	root := t.TempDir()
	distA := filepath.Join(root, "dist-a")
	distB := filepath.Join(root, "dist-b")
	out := filepath.Join(root, "out")
	linuxA := filepath.Join(distA, "linux_amd64", "censorship.so")
	windowsA := filepath.Join(distA, "windows_amd64", "censorship.dll")
	for _, library := range []string{linuxA, windowsA} {
		if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(library, []byte("first build"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	oldVersionArchive := filepath.Join(out, "censorship_1.2.2_windows_amd64.zip")
	keep := filepath.Join(out, "keep.txt")
	for path, contents := range map[string][]byte{oldVersionArchive: []byte("old version"), keep: []byte("keep")} {
		if err := os.WriteFile(path, contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := packageExistingArtifacts("1.2.3", distA, out); err != nil {
		t.Fatal(err)
	}

	linuxB := filepath.Join(distB, "linux_amd64", "censorship.so")
	if err := os.MkdirAll(filepath.Dir(linuxB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linuxB, []byte("second build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := packageExistingArtifacts("1.2.3", distB, out); err != nil {
		t.Fatal(err)
	}

	windowsArchive := filepath.Join(out, "censorship_1.2.3_windows_amd64.zip")
	for _, path := range []string{windowsArchive, windowsArchive + ".sha256"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale output %q stat error = %v, want not exist", path, err)
		}
	}
	linuxArchive := filepath.Join(out, "censorship_1.2.3_linux_amd64.zip")
	for _, path := range []string{linuxArchive, linuxArchive + ".sha256"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("current output %q stat error = %v", path, err)
		}
	}
	checksums, err := os.ReadFile(filepath.Join(out, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}  censorship_1\.2\.3_linux_amd64\.zip\n$`).Match(checksums) {
		t.Fatalf("checksums = %q", checksums)
	}
	for path, want := range map[string][]byte{oldVersionArchive: []byte("old version"), keep: []byte("keep")} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read preserved output %q: %v", path, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("preserved output %q = %q, want %q", path, got, want)
		}
	}
}

func TestPackageExistingArtifactsPreservesMissingPlatformOutputDirectories(t *testing.T) {
	root := t.TempDir()
	distA := filepath.Join(root, "dist-a")
	distB := filepath.Join(root, "dist-b")
	out := filepath.Join(root, "out")
	linuxA := filepath.Join(distA, "linux_amd64", "censorship.so")
	windowsA := filepath.Join(distA, "windows_amd64", "censorship.dll")
	for _, library := range []string{linuxA, windowsA} {
		if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(library, []byte("first build"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := packageExistingArtifacts("1.2.3", distA, out); err != nil {
		t.Fatal(err)
	}

	windowsArchive := filepath.Join(out, "censorship_1.2.3_windows_amd64.zip")
	for _, path := range []string{windowsArchive, windowsArchive + ".sha256"} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	linuxB := filepath.Join(distB, "linux_amd64", "censorship.so")
	if err := os.MkdirAll(filepath.Dir(linuxB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linuxB, []byte("second build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := packageExistingArtifacts("1.2.3", distB, out); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{windowsArchive, windowsArchive + ".sha256"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("preserved output directory %q stat error = %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("preserved output %q is not a directory", path)
		}
	}
}

func TestReplaceOutputFileRestoresExistingDestinationAfterInstallFailure(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "output")
	temporary := filepath.Join(dir, "temporary")
	oldContents := []byte("old output")
	if err := os.WriteFile(destination, oldContents, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("new output"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalRename := renameOutputFile
	t.Cleanup(func() { renameOutputFile = originalRename })
	installFailure := fmt.Errorf("install failure")
	calls := 0
	renameOutputFile = func(oldPath, newPath string) error {
		calls++
		if calls == 2 {
			return installFailure
		}
		return originalRename(oldPath, newPath)
	}
	if err := replaceOutputFile(temporary, destination); err == nil || !strings.Contains(err.Error(), installFailure.Error()) {
		t.Fatalf("replace error = %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldContents) {
		t.Fatalf("destination = %q, want %q", got, oldContents)
	}
	if _, err := os.Stat(temporary); err != nil {
		t.Fatalf("temporary = %v, want retained for caller cleanup", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".backup-") {
			t.Fatalf("backup remained after successful rollback: %s", entry.Name())
		}
	}
}

func TestReplaceOutputFileRetainsBackupWhenRollbackFails(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "output")
	temporary := filepath.Join(dir, "temporary")
	oldContents := []byte("old output")
	if err := os.WriteFile(destination, oldContents, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("new output"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalRename := renameOutputFile
	t.Cleanup(func() { renameOutputFile = originalRename })
	installFailure := fmt.Errorf("install failure")
	rollbackFailure := fmt.Errorf("rollback failure")
	calls := 0
	renameOutputFile = func(oldPath, newPath string) error {
		calls++
		switch calls {
		case 2:
			return installFailure
		case 3:
			return rollbackFailure
		default:
			return originalRename(oldPath, newPath)
		}
	}
	replaceErr := replaceOutputFile(temporary, destination)
	if replaceErr == nil || !strings.Contains(replaceErr.Error(), installFailure.Error()) || !strings.Contains(replaceErr.Error(), rollbackFailure.Error()) {
		t.Fatalf("replace error = %v", replaceErr)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination stat = %v, want absent", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var backup string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".backup-") {
			backup = filepath.Join(dir, entry.Name())
		}
	}
	if backup == "" {
		t.Fatal("missing retained rollback backup")
	}
	if !strings.Contains(replaceErr.Error(), fmt.Sprintf("%q", backup)) {
		t.Fatalf("replace error = %v, want recovery path %q", replaceErr, backup)
	}
	got, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldContents) {
		t.Fatalf("backup = %q, want %q", got, oldContents)
	}
}

func TestWriteOutputFilePreservesDestinationOnCallbackFailure(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "output")
	oldContents := []byte("old output")
	if err := os.WriteFile(destination, oldContents, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	callbackErr := errors.New("callback failure")
	err = writeOutputFile(destination, 0o644, func(*os.File) error { return callbackErr })
	if !errors.Is(err, callbackErr) {
		t.Fatalf("writeOutputFile error = %v, want callback failure", err)
	}
	after, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("callback failure replaced destination")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldContents) {
		t.Fatalf("destination = %q, want %q", got, oldContents)
	}
	assertNoReleaseTemporaryEntries(t, dir)
}

func TestWriteOutputFileJoinsDeferredCleanupErrors(t *testing.T) {
	dir := t.TempDir()
	originalClose := closeOutputFile
	originalRemove := removeOutputFile
	t.Cleanup(func() {
		closeOutputFile = originalClose
		removeOutputFile = originalRemove
	})
	closeErr := errors.New("close temporary")
	removeErr := errors.New("remove temporary")
	closeOutputFile = func(file *os.File) error {
		_ = originalClose(file)
		return closeErr
	}
	removeOutputFile = func(path string) error {
		_ = originalRemove(path)
		return removeErr
	}
	callbackErr := errors.New("callback failure")
	err := writeOutputFile(filepath.Join(dir, "output"), 0o644, func(*os.File) error { return callbackErr })
	for _, want := range []error{callbackErr, closeErr, removeErr} {
		if !errors.Is(err, want) {
			t.Fatalf("writeOutputFile error = %v, does not include %v", err, want)
		}
	}
	assertNoReleaseTemporaryEntries(t, dir)
}

func TestWriteOutputFileRejectsDirectoryDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "output")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	err := writeOutputFile(destination, 0o644, func(file *os.File) error {
		_, err := file.Write([]byte("new output"))
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("writeOutputFile error = %v", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("directory destination was replaced")
	}
	assertNoReleaseTemporaryEntries(t, dir)
}

func TestWriteOutputFileRejectsSymlinkDestination(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	destination := filepath.Join(dir, "output")
	oldContents := []byte("old output")
	if err := os.WriteFile(target, oldContents, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), destination); err != nil {
		if os.IsPermission(err) {
			t.Skipf("symlink not permitted: %v", err)
		}
		t.Fatal(err)
	}
	beforeTarget, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	beforeLink, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeOutputFile(destination, 0o644, func(file *os.File) error {
		_, err := file.Write([]byte("new output"))
		return err
	}); err == nil || !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("writeOutputFile error = %v", err)
	}
	afterTarget, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(beforeTarget, afterTarget) {
		t.Fatal("symlink destination changed target inode")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldContents) {
		t.Fatalf("target = %q, want %q", got, oldContents)
	}
	afterLink, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if afterLink.Mode()&os.ModeSymlink == 0 || !os.SameFile(beforeLink, afterLink) {
		t.Fatal("symlink destination directory entry changed")
	}
	assertNoReleaseTemporaryEntries(t, dir)
}

func TestWriteOutputFileLeavesNoTemporaryFilesAfterInstall(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "output")
	if err := os.WriteFile(destination, []byte("old output"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeOutputFile(destination, 0o600, func(file *os.File) error {
		_, err := file.Write([]byte("new output"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new output" {
		t.Fatalf("destination = %q", got)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("destination mode = %v, want 0600", info.Mode().Perm())
		}
	}
	assertNoReleaseTemporaryEntries(t, dir)
}

func TestReserveOutputBackupIncludesCleanupPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	originalClose := closeOutputFile
	t.Cleanup(func() { closeOutputFile = originalClose })
	closeErr := errors.New("close backup")
	closeOutputFile = func(file *os.File) error {
		_ = originalClose(file)
		return closeErr
	}
	_, err := reserveOutputBackup(path)
	if !errors.Is(err, closeErr) || !strings.Contains(err.Error(), ".output.backup-") {
		t.Fatalf("reserveOutputBackup error = %v", err)
	}
	assertNoReleaseTemporaryEntries(t, dir)
}

func TestPackageLibraryClosesWriterAfterOptionalFileFailure(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	library := filepath.Join(dir, "censorship.so")
	if err := os.WriteFile(library, []byte("library"), 0o755); err != nil {
		t.Fatal(err)
	}
	originalClose := closeZipWriter
	originalAdd := addOptionalArchiveFile
	t.Cleanup(func() {
		closeZipWriter = originalClose
		addOptionalArchiveFile = originalAdd
	})
	closeErr := errors.New("close zip writer")
	addErr := errors.New("optional file failure")
	closeCalls := 0
	closeZipWriter = func(writer *zip.Writer) error {
		closeCalls++
		_ = originalClose(writer)
		return closeErr
	}
	addOptionalArchiveFile = func(*zip.Writer, string) error { return addErr }
	err = packageLibrary(library, filepath.Join(dir, "archive.zip"))
	if !errors.Is(err, addErr) || !errors.Is(err, closeErr) {
		t.Fatalf("packageLibrary error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("zip writer close calls = %d, want 1", closeCalls)
	}
	assertNoReleaseTemporaryEntries(t, dir)
}

func assertNoReleaseTemporaryEntries(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") || strings.Contains(entry.Name(), ".backup-") {
			t.Fatalf("unexpected temporary output entry %s", entry.Name())
		}
	}
}

func TestPackageLibraryIsDeterministicAcrossSourceMtimes(t *testing.T) {
	script, err := filepath.Abs("package-release.go")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	library := filepath.Join(tmp, "censorship.so")
	license := filepath.Join(tmp, "LICENSE")
	archive := filepath.Join(tmp, "censorship_1.2.3_linux_amd64.zip")
	checksum := archive + ".sha256"
	for path, contents := range map[string]string{
		library: "fixture library",
		license: "fixture license\n",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	firstModified := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	secondModified := firstModified.Add(6 * time.Second)
	runPackager := func() {
		cmd := exec.Command("go", "run", script, "-version", "1.2.3", "-library", library, "-archive", archive, "-checksum", checksum)
		cmd.Dir = tmp
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("packager: %v\n%s", err, output)
		}
	}
	for _, path := range []string{library, license} {
		if err := os.Chtimes(path, firstModified, firstModified); err != nil {
			t.Fatal(err)
		}
	}
	runPackager()
	firstArchive, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	firstChecksum, err := os.ReadFile(checksum)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{library, license} {
		if err := os.Chtimes(path, secondModified, secondModified); err != nil {
			t.Fatal(err)
		}
	}
	runPackager()
	secondArchive, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	secondChecksum, err := os.ReadFile(checksum)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(secondArchive, firstArchive) {
		t.Fatal("ZIP bytes changed when only source mtimes changed")
	}
	if !bytes.Equal(secondChecksum, firstChecksum) {
		t.Fatal("checksum line changed when only source mtimes changed")
	}

	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	wantModified := time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, file := range reader.File {
		if !file.Modified.Equal(wantModified) {
			t.Fatalf("ZIP entry %q Modified = %s, want %s", file.Name, file.Modified, wantModified)
		}
	}
}

func TestResolveVersionSources(t *testing.T) {
	if got, err := resolveVersion("v1.2.3"); err != nil || got != "1.2.3" {
		t.Fatalf("flag version = %q, %v", got, err)
	}
	t.Setenv("VERSION", "v1.2.3")
	if got, err := resolveVersion(""); err != nil || got != "1.2.3" {
		t.Fatalf("env version = %q, %v", got, err)
	}
	t.Setenv("VERSION", "")
	repo := t.TempDir()
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	runGitTest(t, repo, "tag", "v1.2.3")
	old, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if got, err := resolveVersion(""); err != nil || got != "1.2.3" {
		t.Fatalf("tag version = %q, %v", got, err)
	}
}

func zipNames(files []*zip.File) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Name
	}
	return names
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestPackagerCLIVersionSourcesProduceSameBasename(t *testing.T) {
	script, err := filepath.Abs("package-release.go")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	dist := filepath.Join(tmp, "dist")
	if err := os.MkdirAll(filepath.Join(dist, "linux_amd64"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "linux_amd64", "censorship.so"), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	tagRepo := filepath.Join(tmp, "tag-repo")
	if err := os.MkdirAll(tagRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, tagRepo, "init")
	runGitTest(t, tagRepo, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	runGitTest(t, tagRepo, "tag", "v1.2.3")

	cases := []struct {
		name, dir, envVersion string
		args                  []string
	}{
		{name: "flag", dir: ".", args: []string{"-version", "v1.2.3"}},
		{name: "environment", dir: ".", envVersion: "v1.2.3"},
		{name: "exact tag", dir: tagRepo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(tmp, strings.ReplaceAll(tc.name, " ", "-"))
			args := append([]string{"run", script}, tc.args...)
			args = append(args, "-dist", dist, "-out", out)
			cmd := exec.Command("go", args...)
			cmd.Dir = tc.dir
			cmd.Env = withEnvironment(os.Environ(), "VERSION", tc.envVersion)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("packager: %v\n%s", err, output)
			}
			archive := filepath.Join(out, "censorship_1.2.3_linux_amd64.zip")
			if _, err := os.Stat(archive); err != nil {
				t.Fatal(err)
			}
			checksum, err := os.ReadFile(archive + ".sha256")
			if err != nil {
				t.Fatal(err)
			}
			aggregate, err := os.ReadFile(filepath.Join(out, "checksums.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(checksum, aggregate) {
				t.Fatalf("checksum = %q, aggregate = %q", checksum, aggregate)
			}
		})
	}
}

func TestPackagerRejectsUnsafeVersionBeforeCreatingOutput(t *testing.T) {
	script, err := filepath.Abs("package-release.go")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	dist := filepath.Join(tmp, "dist")
	library := filepath.Join(dist, "linux_amd64", "censorship.so")
	if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(library, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}

	aggregateOut := filepath.Join(tmp, "aggregate")
	runPackager := func(args ...string) error {
		cmd := exec.Command("go", append([]string{"run", script}, args...)...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("packager unexpectedly succeeded:\n%s", output)
		}
		return err
	}
	if err := runPackager("-version", "foo/bar", "-dist", dist, "-out", aggregateOut); err == nil {
		t.Fatal("aggregate packaging accepted unsafe version")
	}
	if entries, err := os.ReadDir(aggregateOut); err == nil && len(entries) != 0 {
		t.Fatalf("aggregate output = %#v, want absent or empty", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("inspect aggregate output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(aggregateOut, "censorship_foo")); !os.IsNotExist(err) {
		t.Fatalf("unsafe nested output exists: %v", err)
	}

	directArchive := filepath.Join(tmp, "direct", "censorship.zip")
	directChecksum := directArchive + ".sha256"
	if err := runPackager("-version", "foo/bar", "-library", library, "-archive", directArchive, "-checksum", directChecksum); err == nil {
		t.Fatal("direct packaging accepted unsafe version")
	}
	for _, path := range []string{directArchive, directChecksum} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unsafe direct output %s exists: %v", path, err)
		}
	}
}

func TestPackagerRejectsWhitespaceVersionsBeforeChangingOutputs(t *testing.T) {
	script, err := filepath.Abs("package-release.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "leading space", value: " v1.2.3"},
		{name: "trailing space", value: "v1.2.3 "},
		{name: "whitespace only", value: " "},
		{name: "leading newline", value: "\nv1.2.3"},
		{name: "trailing newline", value: "v1.2.3\n"},
		{name: "newline only", value: "\n"},
	} {
		t.Run("flag/"+tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			library := filepath.Join(tmp, "censorship.so")
			archive := filepath.Join(tmp, "censorship.zip")
			checksum := archive + ".sha256"
			for path, contents := range map[string]string{
				library:  "library contents",
				archive:  "archive contents",
				checksum: "checksum contents",
			} {
				if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotFiles(t, library, archive, checksum)

			cmd := exec.Command("go", "run", script, "-version", tc.value, "-library", library, "-archive", archive, "-checksum", checksum)
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("packager accepted %q:\n%s", tc.value, output)
			}
			assertFilesUnchanged(t, before)
		})

		t.Run("environment/"+tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			dist := filepath.Join(tmp, "dist")
			library := filepath.Join(dist, "linux_amd64", "censorship.so")
			if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(library, []byte("library contents"), 0o755); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(tmp, "out")

			cmd := exec.Command("go", "run", script, "-dist", dist, "-out", out)
			cmd.Env = withEnvironment(os.Environ(), "VERSION", tc.value)
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("packager accepted %q:\n%s", tc.value, output)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("whitespace VERSION created output %s: %v", out, err)
			}
		})
	}
}

func TestPackagerRejectsMissingOrBareVersionBeforeChangingOutputs(t *testing.T) {
	script, err := filepath.Abs("package-release.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "missing"},
		{name: "bare v", args: []string{"-version", "v"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			library := filepath.Join(tmp, "censorship.so")
			archive := filepath.Join(tmp, "censorship.zip")
			checksum := filepath.Join(tmp, "censorship.zip.sha256")
			for path, contents := range map[string]string{
				library:  "library contents",
				archive:  "archive contents",
				checksum: "checksum contents",
			} {
				if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotFiles(t, library, archive, checksum)

			args := append([]string{"run", script}, tc.args...)
			args = append(args, "-library", library, "-archive", archive, "-checksum", checksum)
			cmd := exec.Command("go", args...)
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("packager unexpectedly succeeded:\n%s", output)
			}
			assertFilesUnchanged(t, before)
		})
	}
}

func TestPackagerRejectsPathCollisionsBeforeChangingFiles(t *testing.T) {
	script, err := filepath.Abs("package-release.go")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		paths func(t *testing.T, dir string) (string, string, string)
	}{
		{
			name: "library archive equal",
			paths: func(_ *testing.T, dir string) (string, string, string) {
				return filepath.Join(dir, "library.so"), filepath.Join(dir, "library.so"), filepath.Join(dir, "checksum.sha256")
			},
		},
		{
			name: "library checksum equal",
			paths: func(_ *testing.T, dir string) (string, string, string) {
				return filepath.Join(dir, "library.so"), filepath.Join(dir, "archive.zip"), filepath.Join(dir, "library.so")
			},
		},
		{
			name: "archive checksum equal",
			paths: func(_ *testing.T, dir string) (string, string, string) {
				return filepath.Join(dir, "library.so"), filepath.Join(dir, "archive.zip"), filepath.Join(dir, "archive.zip")
			},
		},
		{
			name: "library archive lexical alias",
			paths: func(_ *testing.T, dir string) (string, string, string) {
				return filepath.Join(dir, "library.so"), "library.so", filepath.Join(dir, "checksum.sha256")
			},
		},
		{
			name: "library checksum lexical alias",
			paths: func(_ *testing.T, dir string) (string, string, string) {
				return filepath.Join(dir, "library.so"), filepath.Join(dir, "archive.zip"), "library.so"
			},
		},
		{
			name: "archive checksum lexical alias",
			paths: func(_ *testing.T, dir string) (string, string, string) {
				return filepath.Join(dir, "library.so"), filepath.Join(dir, "archive.zip"), "archive.zip"
			},
		},
		{
			name: "library archive filesystem alias",
			paths: func(t *testing.T, dir string) (string, string, string) {
				library := filepath.Join(dir, "library.so")
				alias := filepath.Join(dir, "library-link.so")
				if err := os.Link(library, alias); err != nil {
					t.Fatal(err)
				}
				return library, alias, filepath.Join(dir, "checksum.sha256")
			},
		},
		{
			name: "library checksum filesystem alias",
			paths: func(t *testing.T, dir string) (string, string, string) {
				library := filepath.Join(dir, "library.so")
				alias := filepath.Join(dir, "library-link.so")
				if err := os.Link(library, alias); err != nil {
					t.Fatal(err)
				}
				return library, filepath.Join(dir, "archive.zip"), alias
			},
		},
		{
			name: "archive checksum filesystem alias",
			paths: func(t *testing.T, dir string) (string, string, string) {
				archive := filepath.Join(dir, "archive.zip")
				alias := filepath.Join(dir, "archive-link.zip")
				if err := os.Link(archive, alias); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(dir, "library.so"), archive, alias
			},
		},
		{
			name: "checksum repository LICENSE",
			paths: func(_ *testing.T, dir string) (string, string, string) {
				return filepath.Join(dir, "library.so"), filepath.Join(dir, "archive.zip"), filepath.Join(dir, "LICENSE")
			},
		},
		{
			name: "archive repository LICENSE",
			paths: func(_ *testing.T, dir string) (string, string, string) {
				return filepath.Join(dir, "library.so"), filepath.Join(dir, "LICENSE"), filepath.Join(dir, "checksum.sha256")
			},
		},
		{
			name: "checksum dangling symlink to archive",
			paths: func(t *testing.T, dir string) (string, string, string) {
				archive := filepath.Join(dir, "archive.zip")
				checksum := filepath.Join(dir, "checksum.sha256")
				if err := os.Remove(archive); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(checksum); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Base(archive), checksum); err != nil {
					if os.IsPermission(err) {
						t.Skipf("symlink not permitted: %v", err)
					}
					t.Fatal(err)
				}
				return filepath.Join(dir, "library.so"), archive, checksum
			},
		},
		{
			name: "LICENSE dangling symlink to archive",
			paths: func(t *testing.T, dir string) (string, string, string) {
				archive := filepath.Join(dir, "archive.zip")
				license := filepath.Join(dir, "LICENSE")
				if err := os.Remove(archive); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(license); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Base(archive), license); err != nil {
					if os.IsPermission(err) {
						t.Skipf("symlink not permitted: %v", err)
					}
					t.Fatal(err)
				}
				return filepath.Join(dir, "library.so"), archive, filepath.Join(dir, "checksum.sha256")
			},
		},
		{
			name: "absent case-only output aliases",
			paths: func(t *testing.T, dir string) (string, string, string) {
				out := filepath.Join(dir, "out")
				if err := os.MkdirAll(out, 0o755); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(dir, "library.so"), filepath.Join(out, "Archive.zip"), filepath.Join(out, "archive.zip")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			for path, contents := range map[string]string{
				filepath.Join(tmp, "library.so"):      "library contents",
				filepath.Join(tmp, "LICENSE"):         "license contents",
				filepath.Join(tmp, "archive.zip"):     "archive contents",
				filepath.Join(tmp, "checksum.sha256"): "checksum contents",
			} {
				if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			library, archive, checksum := tc.paths(t, tmp)
			before := snapshotFiles(t,
				filepath.Join(tmp, "library.so"),
				filepath.Join(tmp, "LICENSE"),
				filepath.Join(tmp, "archive.zip"),
				filepath.Join(tmp, "checksum.sha256"),
				filepath.Join(tmp, "library-link.so"),
				filepath.Join(tmp, "archive-link.zip"),
				library,
				archive,
				checksum,
			)

			cmd := exec.Command("go", "run", script, "-version", "1.2.3", "-library", library, "-archive", archive, "-checksum", checksum)
			cmd.Dir = tmp
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("packager unexpectedly succeeded:\n%s", output)
			}
			assertFilesUnchanged(t, before)
		})
	}
}

func TestValidateDirectPackagePathsRejectsAbsentOutputAliasesThroughJunction(t *testing.T) {
	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(outside, "censorship.so")
	if err := os.WriteFile(library, []byte("library contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "linked")
	linkPackageDirectory(t, outside, link)
	archive := filepath.Join(link, "censorship.zip")
	checksum := filepath.Join(outside, "censorship.zip")

	if _, _, _, err := validateDirectPackagePaths(library, archive, checksum); err == nil {
		t.Fatal("absent archive and checksum aliases accepted")
	}
}

func TestPathsAliasCaseRules(t *testing.T) {
	t.Run("existing case-distinct files", func(t *testing.T) {
		dir := t.TempDir()
		first := filepath.Join(dir, "Artifact")
		second := filepath.Join(dir, "artifact")
		for _, path := range []string{first, second} {
			if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		firstInfo, err := os.Stat(first)
		if err != nil {
			t.Fatal(err)
		}
		secondInfo, err := os.Stat(second)
		if err != nil {
			t.Fatal(err)
		}
		if os.SameFile(firstInfo, secondInfo) {
			t.Skip("filesystem treats case-distinct paths as one file")
		}
		same, err := pathsAlias(first, second)
		if err != nil {
			t.Fatal(err)
		}
		if same {
			t.Fatal("case-distinct existing files were treated as aliases")
		}
	})

	dir := t.TempDir()
	asciiFirst := filepath.Join(dir, "Archive.zip")
	asciiSecond := filepath.Join(dir, "archive.zip")
	if same, err := pathsAlias(asciiFirst, asciiSecond); err != nil {
		t.Fatal(err)
	} else if !same {
		t.Fatal("absent ASCII case-only suffixes were not treated as aliases")
	}

	multibyteFirst := filepath.Join(dir, "é.zip")
	multibyteSecond := filepath.Join(dir, "è.zip")
	if same, err := pathsAlias(multibyteFirst, multibyteSecond); err != nil {
		t.Fatal(err)
	} else if same {
		t.Fatal("same-length multibyte absent suffixes were treated as aliases")
	}

	unicodeFirst := filepath.Join(dir, "Kelvin.zip")
	unicodeSecond := filepath.Join(dir, "Kelvin.zip")
	if !strings.EqualFold(filepath.Base(unicodeFirst), filepath.Base(unicodeSecond)) {
		t.Fatal("Unicode case-fold fixture is not equivalent")
	}
	if same, err := pathsAlias(unicodeFirst, unicodeSecond); err != nil {
		t.Fatal(err)
	} else if same {
		t.Fatal("Unicode fold-equivalent absent suffixes were treated as aliases")
	}
}

func linkPackageDirectory(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Fatalf("create junction: %v\n%s", err, output)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

type fileSnapshot struct {
	exists     bool
	symlink    bool
	contents   []byte
	linkTarget string
	modTime    int64
}

func snapshotFiles(t *testing.T, paths ...string) map[string]fileSnapshot {
	t.Helper()
	out := make(map[string]fileSnapshot, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			out[path] = fileSnapshot{}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		snapshot := fileSnapshot{
			exists:  true,
			symlink: info.Mode()&os.ModeSymlink != 0,
			modTime: info.ModTime().UnixNano(),
		}
		if snapshot.symlink {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.linkTarget = linkTarget
		} else {
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.contents = contents
		}
		out[path] = snapshot
	}
	return out
}

func assertFilesUnchanged(t *testing.T, before map[string]fileSnapshot) {
	t.Helper()
	for path, want := range before {
		info, err := os.Lstat(path)
		if !want.exists {
			if err == nil {
				t.Fatalf("path was created: %s", path)
			}
			if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("inspect %s: %v", path, err)
		}
		if got := info.ModTime().UnixNano(); got != want.modTime {
			t.Fatalf("timestamp changed for %s: %d, want %d", path, got, want.modTime)
		}
		if got := info.Mode()&os.ModeSymlink != 0; got != want.symlink {
			t.Fatalf("symlink state changed for %s", path)
		}
		if want.symlink {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				t.Fatal(err)
			}
			if linkTarget != want.linkTarget {
				t.Fatalf("symlink target changed for %s: %q, want %q", path, linkTarget, want.linkTarget)
			}
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !bytes.Equal(contents, want.contents) {
			t.Fatalf("contents changed for %s: %q, want %q", path, contents, want.contents)
		}
	}
}

func TestPackageExistingArtifactsRejectsArchiveAliasWithoutChangingFiles(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	out := filepath.Join(t.TempDir(), "out")
	artifact := artifactSpec{osName: "linux", arch: "amd64"}
	library := artifact.binaryPath(dist)
	if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(library, []byte("library contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(out, "censorship_1.2.3_linux_amd64.zip")
	if err := os.Link(library, archive); err != nil {
		t.Fatal(err)
	}
	checksum := archive + ".sha256"
	aggregate := filepath.Join(out, "checksums.txt")
	before := snapshotFiles(t, library, archive, checksum, aggregate)

	if err := packageExistingArtifacts("1.2.3", dist, out); err == nil {
		t.Fatal("aggregate packaging accepted an archive alias of its input library")
	}
	assertFilesUnchanged(t, before)
}

func TestPackageExistingArtifactsRejectsLateArchiveAliasWithoutChangingFiles(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	out := filepath.Join(t.TempDir(), "out")
	artifacts := artifactSpecs()[:2]
	libraries := make([]string, len(artifacts))
	archives := make([]string, len(artifacts))
	checksums := make([]string, len(artifacts))
	for i, artifact := range artifacts {
		libraries[i] = artifact.binaryPath(dist)
		if err := os.MkdirAll(filepath.Dir(libraries[i]), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(libraries[i], []byte(artifact.osName+artifact.arch), 0o755); err != nil {
			t.Fatal(err)
		}
		archives[i] = filepath.Join(out, fmt.Sprintf("%s_%s_%s_%s.zip", pluginName, "1.2.3", artifact.osName, artifact.arch))
		checksums[i] = archives[i] + ".sha256"
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archives[0], []byte("existing archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(libraries[1], archives[1]); err != nil {
		t.Fatal(err)
	}
	aggregate := filepath.Join(out, "checksums.txt")
	before := snapshotFiles(t,
		libraries[0], libraries[1],
		archives[0], archives[1],
		checksums[0], checksums[1],
		aggregate,
	)

	if err := packageExistingArtifacts("1.2.3", dist, out); err == nil {
		t.Fatal("aggregate packaging accepted a late archive alias of its input library")
	}
	assertFilesUnchanged(t, before)
}

func TestPackageExistingArtifactsRejectsCrossArtifactAndAggregateAliasesWithoutChangingFiles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alias func(*testing.T, aggregateAliasFixture)
	}{
		{
			name: "first archive hard-linked to second library",
			alias: func(t *testing.T, fixture aggregateAliasFixture) {
				t.Helper()
				if err := os.Link(fixture.libraries[1], fixture.archives[0]); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "archives from different artifacts hard-linked",
			alias: func(t *testing.T, fixture aggregateAliasFixture) {
				t.Helper()
				if err := os.WriteFile(fixture.archives[0], []byte("existing archive"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(fixture.archives[0], fixture.archives[1]); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checksums hard-linked to library",
			alias: func(t *testing.T, fixture aggregateAliasFixture) {
				t.Helper()
				if err := os.Link(fixture.libraries[0], fixture.aggregate); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checksums hard-linked to archive",
			alias: func(t *testing.T, fixture aggregateAliasFixture) {
				t.Helper()
				if err := os.WriteFile(fixture.archives[0], []byte("existing archive"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(fixture.archives[0], fixture.aggregate); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "checksums hard-linked to repository LICENSE",
			alias: func(t *testing.T, fixture aggregateAliasFixture) {
				t.Helper()
				license := filepath.Join(fixture.root, "LICENSE")
				if err := os.WriteFile(license, []byte("license contents"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(license, fixture.aggregate); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newAggregateAliasFixture(t)
			old, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(fixture.root); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(old) })
			tc.alias(t, fixture)
			before := snapshotAggregateAliasFixture(t, fixture)

			if err := packageExistingArtifacts("1.2.3", fixture.dist, fixture.out); err == nil {
				t.Errorf("aggregate packaging accepted %s", tc.name)
			}
			assertFilesUnchanged(t, before)
		})
	}
}

type aggregateAliasFixture struct {
	root      string
	dist      string
	out       string
	libraries []string
	archives  []string
	checksums []string
	aggregate string
}

func newAggregateAliasFixture(t *testing.T) aggregateAliasFixture {
	t.Helper()
	root := t.TempDir()
	fixture := aggregateAliasFixture{
		root: root,
		dist: filepath.Join(root, "dist"),
		out:  filepath.Join(root, "out"),
	}
	for _, artifact := range artifactSpecs()[:2] {
		library := artifact.binaryPath(fixture.dist)
		if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(library, []byte(artifact.osName+artifact.arch), 0o755); err != nil {
			t.Fatal(err)
		}
		archive := filepath.Join(fixture.out, fmt.Sprintf("%s_%s_%s_%s.zip", pluginName, "1.2.3", artifact.osName, artifact.arch))
		fixture.libraries = append(fixture.libraries, library)
		fixture.archives = append(fixture.archives, archive)
		fixture.checksums = append(fixture.checksums, archive+".sha256")
	}
	if err := os.MkdirAll(fixture.out, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture.aggregate = filepath.Join(fixture.out, "checksums.txt")
	return fixture
}

func snapshotAggregateAliasFixture(t *testing.T, fixture aggregateAliasFixture) map[string]fileSnapshot {
	t.Helper()
	paths := append([]string(nil), fixture.libraries...)
	paths = append(paths, fixture.archives...)
	paths = append(paths, fixture.checksums...)
	paths = append(paths, fixture.aggregate, filepath.Join(fixture.root, "LICENSE"))
	return snapshotFiles(t, paths...)
}

func TestPackageExistingArtifactsHashesEachArchiveOnce(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	out := filepath.Join(t.TempDir(), "out")
	artifacts := []artifactSpec{
		{osName: "linux", arch: "amd64"},
		{osName: "windows", arch: "amd64"},
	}
	for _, artifact := range artifacts {
		path := artifact.binaryPath(dist)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(artifact.osName+artifact.arch), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	originalSHA256File := sha256File
	t.Cleanup(func() { sha256File = originalSHA256File })
	calls := 0
	sha256File = func(path string) (string, error) {
		calls++
		return originalSHA256File(path)
	}

	if err := packageExistingArtifacts("1.2.3", dist, out); err != nil {
		t.Fatal(err)
	}
	if calls != len(artifacts) {
		t.Fatalf("sha256File calls = %d, want %d", calls, len(artifacts))
	}

	aggregate, err := os.ReadFile(filepath.Join(out, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		archive := filepath.Join(out, fmt.Sprintf("%s_%s_%s_%s.zip", pluginName, "1.2.3", artifact.osName, artifact.arch))
		individual, err := os.ReadFile(archive + ".sha256")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(aggregate), string(individual)) {
			t.Fatalf("aggregate checksum does not retain %q", individual)
		}
	}
}

func withEnvironment(base []string, key, value string) []string {
	out := make([]string, 0, len(base)+1)
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(name, key) {
			out = append(out, entry)
		}
	}
	return append(out, key+"="+value)
}

func withoutEnvironment(base []string, keys ...string) []string {
	out := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		remove := false
		for _, key := range keys {
			if strings.EqualFold(name, key) {
				remove = true
				break
			}
		}
		if !remove {
			out = append(out, entry)
		}
	}
	return out
}

func TestMakeBuildIgnoresTargetOverrides(t *testing.T) {
	forcedOS := "plan9"
	if runtime.GOOS == forcedOS {
		forcedOS = "linux"
	}
	forcedArch := "386"
	if runtime.GOARCH == forcedArch {
		forcedArch = "amd64"
	}

	cmd := exec.Command("make", "-n", "build", "GOOS="+forcedOS, "GOARCH="+forcedArch)
	cmd.Dir = filepath.Join("..", "..")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n build: %v\n%s", err, output)
	}
	want := `build-platform GOOS="` + runtime.GOOS + `" GOARCH="` + runtime.GOARCH + `"`
	if !strings.Contains(string(output), want) {
		t.Fatalf("make build did not select host tuple %s/%s:\n%s", runtime.GOOS, runtime.GOARCH, output)
	}

	cmd = exec.Command("make", "-n", "package-platform", "GOOS=linux", "GOARCH=amd64", "VERSION=v1.2.3")
	cmd.Dir = filepath.Join("..", "..")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n package-platform: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `-version "$PACKAGER_VERSION" -library`) {
		t.Fatalf("make package-platform did not read the raw version from the environment:\n%s", output)
	}

	cmd = exec.Command("make", "-n", "package", "GOOS=linux", "GOARCH=amd64", "VERSION=vv")
	cmd.Dir = filepath.Join("..", "..")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n package VERSION=vv: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `-version "$PACKAGER_VERSION" -library "dist/linux_amd64/censorship.so" -archive "dist/censorship_v_linux_amd64.zip"`) {
		t.Fatalf("make package did not carry raw vv to the packager and use v in the artifact name:\n%s", output)
	}

	cmd = exec.Command("make", "-n", "package", "VERSION=vv")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = withEnvironment(withEnvironment(os.Environ(), "GOOS", ""), "GOARCH", "")
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n package VERSION=vv without target tuple: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `-dist dist -out dist -version "$PACKAGER_VERSION"`) {
		t.Fatalf("make package aggregate command did not carry raw vv to the packager:\n%s", output)
	}
}

func TestMakeMissingVersionUsesDevelopmentDefault(t *testing.T) {
	makefile, err := filepath.Abs(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("make", "-f", makefile, "validate-version")
	cmd.Dir = t.TempDir()
	cmd.Env = withoutEnvironment(os.Environ(), "VERSION", "PACKAGER_VERSION", "NORMALIZED_VERSION")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("make validate-version without VERSION: %v\n%s", err, output)
	}
}

func TestMakeVersionValidationContract(t *testing.T) {
	makefile, err := filepath.Abs(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		value       string
		sentinel    string
		environment bool
	}{
		{value: `v1";touch VERSION_INJECTION_SENTINEL;version="1`, sentinel: "VERSION_INJECTION_SENTINEL"},
		{value: `$(shell touch VERSION_MAKE_SENTINEL)`, sentinel: "VERSION_MAKE_SENTINEL"},
		{value: ""},
		{value: " v1.2.3", environment: true},
		{value: "v1.2.3 ", environment: true},
		{value: " ", environment: true},
		{value: "\nv1.2.3", environment: true},
		{value: "v1.2.3\n", environment: true},
		{value: "\n", environment: true},
		{value: "v1\nprintf injected", sentinel: ""},
		{value: "v1 whitespace", sentinel: ""},
		{value: "v1/path", sentinel: ""},
	} {
		t.Run("rejects unsafe VERSION", func(t *testing.T) {
			tmp := t.TempDir()
			args := []string{"-f", makefile, "validate-version"}
			if !tc.environment {
				args = append(args, "VERSION="+tc.value)
			}
			cmd := exec.Command("make", args...)
			cmd.Dir = tmp
			if tc.environment {
				cmd.Env = withEnvironment(os.Environ(), "VERSION", tc.value)
			}
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("make validate-version accepted %q:\n%s", tc.value, output)
			}
			if tc.sentinel == "" {
				return
			}
			if _, err := os.Lstat(filepath.Join(tmp, tc.sentinel)); err == nil {
				t.Fatalf("make validate-version created %s for %q", tc.sentinel, tc.value)
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		})
	}

	repo := filepath.Join("..", "..")
	for _, version := range []string{"foo/bar", "v"} {
		t.Run("rejects "+version, func(t *testing.T) {
			cmd := exec.Command("make", "validate-version", "VERSION="+version)
			cmd.Dir = repo
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("make validate-version accepted %q:\n%s", version, output)
			}
		})
	}

	for _, target := range []string{"build-platform", "build", "package-platform", "package"} {
		t.Run(target+" validates before build", func(t *testing.T) {
			cmd := exec.Command("make", "-n", target, "VERSION=foo/bar", "GOOS=linux", "GOARCH=amd64")
			cmd.Dir = repo
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("make -n %s: %v\n%s", target, err, output)
			}
			guard := strings.Index(string(output), "VERSION must normalize to a safe non-empty release version")
			build := strings.Index(string(output), "mkdir -p")
			if guard == -1 || build != -1 && guard > build {
				t.Fatalf("make -n %s does not validate before building:\n%s", target, output)
			}
		})
	}
}

func TestDocumentationSelectorContractIncludesToolResults(t *testing.T) {
	for _, path := range []string{filepath.Join("..", "..", "README.md"), filepath.Join("..", "..", "RELEASE_NOTES.md")} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(contents)
		for _, want := range []string{
			"The signature field and value remain excluded, but a non-null `thoughtSignature`",
			"Scalar Claude user content cannot be stripped to empty",
			"Responses prompt variables and local shell skill descriptions use canonical `user`",
			"Machine arguments, grammar definitions, names, IDs, paths, schema values, and reasoning state remain excluded.",
			"An unknown external hard-link peer of an existing destination is not modified",
			"a user `tool_result`'s string content or nested `text`, `search_result`, and `document` text",
			"`function`, `custom-tool`, `shell`, `apply-patch`, `MCP`, and `program` result-output text",
			"Claude unselected tool-result fields",
			"`program_output.result`",
			"`mcp_call.output`",
			"`mcp_call.error.message`",
			"`mcp_list_tools.error`",
			"`file_search_call.results[*].text`",
			"`code_interpreter_call.outputs[type=logs].logs`",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s omits %q", path, want)
			}
		}
		if strings.Contains(text, "Claude non-typed-text tool results") {
			t.Fatalf("%s excludes selected Claude tool-result text", path)
		}
	}
}

func TestDocumentationDoesNotDenyOptionalLicense(t *testing.T) {
	for _, path := range []string{filepath.Join("..", "..", "README.md"), filepath.Join("..", "..", "RELEASE_NOTES.md")} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(contents)
		if !strings.Contains(text, "Each ZIP contains the platform library and an optional repository `LICENSE` if one exists.") {
			t.Fatalf("%s omits conditional LICENSE packaging", path)
		}
		if strings.Contains(text, "This repository does not add a license file.") {
			t.Fatalf("%s falsely denies optional LICENSE packaging", path)
		}
	}
}

type workflowFile struct {
	On   map[string]yaml.Node   `yaml:"on"`
	Env  map[string]string      `yaml:"env"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowPush struct {
	Branches []string `yaml:"branches"`
	Tags     []string `yaml:"tags"`
}

type workflowJob struct {
	Needs    any    `yaml:"needs"`
	If       string `yaml:"if"`
	Strategy struct {
		Matrix struct {
			Include []workflowPlatform `yaml:"include"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []workflowStep `yaml:"steps"`
}

type workflowPlatform struct {
	GOOS   string `yaml:"GOOS"`
	GOARCH string `yaml:"GOARCH"`
	Runner string `yaml:"runner"`
}

type workflowStep struct {
	ID    string            `yaml:"id"`
	Uses  string            `yaml:"uses"`
	Run   string            `yaml:"run"`
	If    string            `yaml:"if"`
	Shell string            `yaml:"shell"`
	Env   map[string]string `yaml:"env"`
	With  map[string]any    `yaml:"with"`
}

func TestBuildWorkflowContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "workflows", "build.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("workflow YAML: %v", err)
	}
	for _, trigger := range []string{"pull_request", "push", "workflow_dispatch"} {
		if _, ok := workflow.On[trigger]; !ok {
			t.Fatalf("missing trigger %q", trigger)
		}
	}
	var push workflowPush
	pushNode := workflow.On["push"]
	if err := pushNode.Decode(&push); err != nil {
		t.Fatalf("decode push trigger: %v", err)
	}
	if !reflect.DeepEqual(push.Branches, []string{"main"}) || !reflect.DeepEqual(push.Tags, []string{"v*"}) {
		t.Fatalf("push trigger = %#v", push)
	}
	if workflow.Env["PLUGIN_NAME"] != "censorship" {
		t.Fatalf("workflow env = %#v", workflow.Env)
	}

	testRuns := nonemptyRuns(workflow.Jobs["test"].Steps)
	wantTestRuns := []string{
		"make test",
		"go test .github/scripts/package-release.go .github/scripts/package-release_test.go",
		"make race",
		"make vet",
		"make integration",
	}
	if !reflect.DeepEqual(testRuns, wantTestRuns) {
		t.Fatalf("test commands = %#v, want %#v", testRuns, wantTestRuns)
	}

	const buildCondition = "${{ github.event_name != 'pull_request' }}"
	const releaseCondition = "${{ startsWith(github.ref, 'refs/tags/v') }}"
	wantMatrix := []workflowPlatform{
		{GOOS: "linux", GOARCH: "amd64", Runner: "ubuntu-24.04"},
		{GOOS: "linux", GOARCH: "arm64", Runner: "ubuntu-24.04-arm"},
		{GOOS: "darwin", GOARCH: "amd64", Runner: "macos-15-intel"},
		{GOOS: "darwin", GOARCH: "arm64", Runner: "macos-15"},
		{GOOS: "windows", GOARCH: "amd64", Runner: "windows-2025"},
	}
	build := workflow.Jobs["build"]
	if !reflect.DeepEqual(build.Strategy.Matrix.Include, wantMatrix) || !reflect.DeepEqual(normalizeNeeds(build.Needs), []string{"test"}) || build.If != buildCondition {
		t.Fatalf("build job = %#v", build)
	}
	if !hasReleaseMetadata(build) ||
		!jobRunContains(build, "make package") ||
		!jobRunWithShellAndIf(build, "make package", "bash", "${{ matrix.GOOS != 'windows' }}") ||
		!jobRunWithShellAndIf(build, "make package", "msys2 {0}", "${{ matrix.GOOS == 'windows' }}") ||
		!jobRunContains(build, `VERSION="${MAKE_VERSION}"`) ||
		!jobRunContains(build, "GOOS=${{ matrix.GOOS }}") ||
		!jobRunContains(build, "GOARCH=${{ matrix.GOARCH }}") ||
		jobActionWith(build, "msys2/setup-msys2@v2", "path-type") != "inherit" ||
		!jobUses(build, "actions/upload-artifact@v4") ||
		jobActionWithValue(build, "actions/upload-artifact@v4", "compression-level") != 0 ||
		jobActionWith(build, "actions/upload-artifact@v4", "if-no-files-found") != "error" {
		t.Fatal("matrix package/upload contract is incomplete")
	}

	const crossAction = "go-cross/cgo-actions@d0b8f2f2d67923ce9a42d92a7ef0ed1ebd905f0a"
	if got := bytes.Count(raw, []byte("uses: "+crossAction+" # v1")); got != 2 {
		t.Fatalf("pinned cross action lines = %d, want 2", got)
	}
	cross := map[string]struct {
		target, dir, library, archive string
	}{
		"build-windows-arm64": {target: "windows-arm64", dir: "windows_arm64", library: "censorship.dll", archive: "censorship_${VERSION}_windows_arm64.zip"},
		"build-freebsd-amd64": {target: "freebsd-amd64", dir: "freebsd_amd64", library: "censorship.so", archive: "censorship_${VERSION}_freebsd_amd64.zip"},
	}
	for id, tc := range cross {
		job := workflow.Jobs[id]
		libraryPath := "dist/" + tc.dir + "/" + tc.library
		if !reflect.DeepEqual(normalizeNeeds(job.Needs), []string{"test"}) ||
			job.If != buildCondition ||
			!hasReleaseMetadata(job) ||
			!jobUses(job, crossAction) ||
			jobActionEnv(job, crossAction, "GOFLAGS") != "-trimpath -buildmode=c-shared" ||
			jobActionWith(job, crossAction, "dir") != "." ||
			jobActionWith(job, crossAction, "packages") != "." ||
			jobActionWith(job, crossAction, "targets") != tc.target ||
			jobActionWith(job, crossAction, "out-dir") != "dist/"+tc.dir ||
			jobActionWith(job, crossAction, "output") != tc.library ||
			jobActionWith(job, crossAction, "flags") != "-ldflags=-s -w" ||
			jobActionWith(job, crossAction, "x-flags") != "main.pluginVersion=${{ steps.release_metadata.outputs.version }}" ||
			!jobRunContains(job, "-version \"${MAKE_VERSION}\"") ||
			!jobRunContains(job, "go run ./.github/scripts/package-release.go") ||
			!jobRunContains(job, "-library \""+libraryPath+"\"") ||
			!jobRunContains(job, "-archive \"dist/"+tc.archive+"\"") ||
			!jobRunContains(job, "-checksum \"dist/"+tc.archive+".sha256\"") ||
			!jobUses(job, "actions/upload-artifact@v4") ||
			jobActionWithValue(job, "actions/upload-artifact@v4", "compression-level") != 0 ||
			jobActionWith(job, "actions/upload-artifact@v4", "if-no-files-found") != "error" {
			t.Fatalf("cross job %s = %#v", id, job)
		}
	}

	release := workflow.Jobs["release"]
	wantNeeds := []string{"build", "build-freebsd-amd64", "build-windows-arm64"}
	gotNeeds := normalizeNeeds(release.Needs)
	sort.Strings(gotNeeds)
	if !reflect.DeepEqual(gotNeeds, wantNeeds) ||
		release.If != releaseCondition ||
		!jobUses(release, "actions/checkout@v5") ||
		jobActionWith(release, "actions/download-artifact@v4", "path") != "release" ||
		!jobActionWithBool(release, "actions/download-artifact@v4", "merge-multiple") {
		t.Fatalf("release job = %#v", release)
	}

	var releaseRun string
	for _, step := range release.Steps {
		if strings.Contains(step.Run, "cat release/*.sha256 | sort > release/checksums.txt") {
			releaseRun = step.Run
			break
		}
	}
	for _, want := range []string{
		"--json isDraft --jq .isDraft",
		"--draft",
		`if [[ "${release_state}" == "true" ]]`,
		`gh release edit "$tag" --draft=false`,
		`gh release download "$tag"`,
		"diff -qr release",
		"sha256sum --check checksums.txt",
		"*.zip.sha256",
		"verify_release() {",
		"mktemp -d",
		`trap 'rm -rf "$verify_dir"' EXIT`,
	} {
		if !strings.Contains(releaseRun, want) {
			t.Fatalf("release run omits %q:\n%s", want, releaseRun)
		}
	}

	const draftBranch = "if [[ \"${release_state}\" == \"true\" ]]; then\n" +
		"    gh release upload \"$tag\" release/*.zip release/*.sha256 release/checksums.txt --clobber\n" +
		"    verify_release\n" +
		"    gh release edit \"$tag\" --draft=false\n" +
		"  else\n" +
		"    verify_release\n" +
		"  fi"
	if !strings.Contains(releaseRun, draftBranch) {
		t.Fatalf("release run does not isolate the draft and published branches:\n%s", releaseRun)
	}
	if strings.Count(releaseRun, "gh release upload") != 1 || strings.Count(releaseRun, "--clobber") != 1 {
		t.Fatalf("release run allows upload or --clobber outside the draft branch:\n%s", releaseRun)
	}

	const newReleaseBranch = "else\n" +
		"  gh release create \"$tag\" release/*.zip release/*.sha256 release/checksums.txt --draft --verify-tag --notes-file RELEASE_NOTES.md\n" +
		"  verify_release\n" +
		"  gh release edit \"$tag\" --draft=false\n" +
		"fi"
	if !strings.Contains(releaseRun, newReleaseBranch) {
		t.Fatalf("release run does not create, verify, and publish a new draft:\n%s", releaseRun)
	}
}

func nonemptyRuns(steps []workflowStep) []string {
	var runs []string
	for _, step := range steps {
		if run := strings.TrimSpace(step.Run); run != "" {
			runs = append(runs, run)
		}
	}
	return runs
}

func normalizeNeeds(value any) []string {
	switch needs := value.(type) {
	case string:
		return []string{needs}
	case []any:
		out := make([]string, 0, len(needs))
		for _, item := range needs {
			need, ok := item.(string)
			if !ok {
				return []string{"<invalid>"}
			}
			out = append(out, need)
		}
		return out
	case nil:
		return nil
	default:
		return []string{"<invalid>"}
	}
}

func jobRunContains(job workflowJob, fragment string) bool {
	for _, step := range job.Steps {
		if strings.Contains(step.Run, fragment) {
			return true
		}
	}
	return false
}

func jobRunWithShellAndIf(job workflowJob, fragment, shell, condition string) bool {
	for _, step := range job.Steps {
		if strings.Contains(step.Run, fragment) && step.Shell == shell && step.If == condition {
			return true
		}
	}
	return false
}

func hasReleaseMetadata(job workflowJob) bool {
	for _, step := range job.Steps {
		if step.ID != "release_metadata" {
			continue
		}
		return strings.Contains(step.Run, `VERSION="${GITHUB_REF_NAME#v}"`) &&
			strings.Contains(step.Run, `MAKE_VERSION="${GITHUB_REF_NAME}"`) &&
			strings.Contains(step.Run, `VERSION="0.0.0-dev"`) &&
			strings.Contains(step.Run, `MAKE_VERSION="0.0.0-dev"`) &&
			strings.Contains(step.Run, `echo "VERSION=${VERSION}" >> "${GITHUB_ENV}"`) &&
			strings.Contains(step.Run, `echo "MAKE_VERSION=${MAKE_VERSION}" >> "${GITHUB_ENV}"`) &&
			strings.Contains(step.Run, `echo "version=${VERSION}" >> "${GITHUB_OUTPUT}"`)
	}
	return false
}

func jobActionEnv(job workflowJob, action, key string) string {
	for _, step := range job.Steps {
		if step.Uses == action {
			return step.Env[key]
		}
	}
	return ""
}

func jobActionWith(job workflowJob, action, key string) string {
	for _, step := range job.Steps {
		if step.Uses == action {
			value, _ := step.With[key].(string)
			return value
		}
	}
	return ""
}

func jobActionWithValue(job workflowJob, action, key string) any {
	for _, step := range job.Steps {
		if step.Uses == action {
			return step.With[key]
		}
	}
	return nil
}

func jobActionWithBool(job workflowJob, action, key string) bool {
	for _, step := range job.Steps {
		if step.Uses == action {
			value, _ := step.With[key].(bool)
			return value
		}
	}
	return false
}

func jobUses(job workflowJob, action string) bool {
	for _, step := range job.Steps {
		if step.Uses == action {
			return true
		}
	}
	return false
}
