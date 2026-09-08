package main

import (
	"archive/zip"
	"bytes"
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			for path, contents := range map[string]string{
				filepath.Join(tmp, "library.so"):      "library contents",
				filepath.Join(tmp, "archive.zip"):     "archive contents",
				filepath.Join(tmp, "checksum.sha256"): "checksum contents",
			} {
				if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			library, archive, checksum := tc.paths(t, tmp)
			before := snapshotFiles(t, filepath.Join(tmp, "library.so"), filepath.Join(tmp, "archive.zip"), filepath.Join(tmp, "checksum.sha256"), filepath.Join(tmp, "library-link.so"), filepath.Join(tmp, "archive-link.zip"))

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
	contents []byte
	modTime  int64
}

func snapshotFiles(t *testing.T, paths ...string) map[string]fileSnapshot {
	t.Helper()
	out := make(map[string]fileSnapshot, len(paths))
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		out[path] = fileSnapshot{contents: contents, modTime: info.ModTime().UnixNano()}
	}
	return out
}

func assertFilesUnchanged(t *testing.T, before map[string]fileSnapshot) {
	t.Helper()
	for path, want := range before {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !bytes.Equal(got, want.contents) {
			t.Fatalf("contents changed for %s: %q, want %q", path, got, want.contents)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.ModTime().UnixNano(); got != want.modTime {
			t.Fatalf("timestamp changed for %s: %d, want %d", path, got, want.modTime)
		}
	}
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
	if !strings.Contains(string(output), `-version "1.2.3" -library`) {
		t.Fatalf("make package-platform did not pass normalized version:\n%s", output)
	}
}

func TestMakeVersionValidationContract(t *testing.T) {
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
			"a user `tool_result`'s string content or nested `text`, `search_result`, and `document` text",
			"`function`, `custom-tool`, `shell`, `apply-patch`, `MCP`, and `program` result-output text",
			"Claude unselected tool-result fields",
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
			!jobRunContains(job, "-version \"${VERSION}\"") ||
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
		!jobActionWithBool(release, "actions/download-artifact@v4", "merge-multiple") ||
		!jobRunContains(release, "cat release/*.sha256 | sort > release/checksums.txt") ||
		!jobRunContains(release, "gh release upload") ||
		!jobRunContains(release, "--clobber") ||
		!jobRunContains(release, "gh release create") ||
		!jobRunContains(release, "--verify-tag") ||
		!jobRunContains(release, `gh release edit "$tag" --notes-file RELEASE_NOTES.md`) ||
		!jobRunContains(release, "--notes-file RELEASE_NOTES.md") {
		t.Fatalf("release job = %#v", release)
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
