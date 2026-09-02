package main

import (
	"archive/zip"
	"bytes"
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
		!jobUses(build, "actions/upload-artifact@v4") {
		t.Fatal("matrix package/upload contract is incomplete")
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
			!jobUses(job, "go-cross/cgo-actions@v1") ||
			jobActionEnv(job, "go-cross/cgo-actions@v1", "GOFLAGS") != "-trimpath -buildmode=c-shared" ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "dir") != "." ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "packages") != "." ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "targets") != tc.target ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "out-dir") != "dist/"+tc.dir ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "output") != tc.library ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "flags") != "-ldflags=-s -w" ||
			jobActionWith(job, "go-cross/cgo-actions@v1", "x-flags") != "main.pluginVersion=${{ steps.release_metadata.outputs.version }}" ||
			!jobRunContains(job, "go run ./.github/scripts/package-release.go") ||
			!jobRunContains(job, "-library \""+libraryPath+"\"") ||
			!jobRunContains(job, "-archive \"dist/"+tc.archive+"\"") ||
			!jobRunContains(job, "-checksum \"dist/"+tc.archive+".sha256\"") ||
			!jobUses(job, "actions/upload-artifact@v4") {
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
