package main

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
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
	if err := writeChecksum(checksum, archive); err != nil {
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
