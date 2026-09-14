package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestRunnerPathsStayUnderIntegrationRoot(t *testing.T) {
	root := t.TempDir()
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.checkout, paths.mapperCheckout, paths.bin, paths.run} {
		rel, err := filepath.Rel(paths.integrationRoot, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("path %q escapes %q", path, paths.integrationRoot)
		}
	}
	if runtime.GOOS == "windows" && filepath.Ext(paths.bin) != ".exe" {
		t.Fatalf("Windows CPA binary path %q has no .exe extension", paths.bin)
	}
}

func TestPluginExtension(t *testing.T) {
	for goos, want := range map[string]string{"windows": ".dll", "darwin": ".dylib", "linux": ".so", "freebsd": ".so"} {
		got, err := pluginExtension(goos)
		if err != nil || got != want {
			t.Errorf("pluginExtension(%q) = %q, %v; want %q", goos, got, err, want)
		}
	}
	if _, err := pluginExtension("plan9"); err == nil {
		t.Fatal("unsupported GOOS accepted")
	}
}

func TestGoCommandUsesGOOverride(t *testing.T) {
	t.Setenv("GO", "go-wrapper")

	if got := goCommand(); got != "go-wrapper" {
		t.Fatalf("go command = %q, want %q", got, "go-wrapper")
	}
}

func TestGoCommandDefaultsToGoWhenGOIsEmpty(t *testing.T) {
	t.Setenv("GO", "")

	if got := goCommand(); got != "go" {
		t.Fatalf("go command = %q, want %q", got, "go")
	}
}

func TestPinnedCPARevision(t *testing.T) {
	const want = "c76dfd4e0edabab9000628b1560ab8ab379eadb8"
	if cpaSHA != want {
		t.Fatalf("cpaSHA = %q, want %q", cpaSHA, want)
	}
}

func TestPinnedModelMapperRevision(t *testing.T) {
	const want = "8fe4839dd2c39a4b0537447c4ac35a9f1d699bbf"
	if modelMapperSHA != want {
		t.Fatalf("modelMapperSHA = %q, want %q", modelMapperSHA, want)
	}
}

func TestVerifyCheckoutRejectsWrongHEAD(t *testing.T) {
	dir := t.TempDir()
	runGitTest(t, dir, "init")
	runGitTest(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	if err := verifyCheckout(dir, cpaSHA); err == nil {
		t.Fatal("wrong checkout HEAD accepted")
	}
}

func TestVerifyCheckoutRejectsDirtyWorktree(t *testing.T) {
	dir := t.TempDir()
	runGitTest(t, dir, "init")
	tracked := filepath.Join(dir, "tracked.go")
	if err := os.WriteFile(tracked, []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, dir, "add", "tracked.go")
	runGitTest(t, dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture")
	head := strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))
	if err := verifyCheckout(dir, head); err != nil {
		t.Fatalf("clean checkout rejected: %v", err)
	}

	if err := os.WriteFile(tracked, []byte("package changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyCheckout(dir, head); err == nil {
		t.Fatal("tracked modification accepted")
	}
	runGitTest(t, dir, "reset", "--hard", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "untracked.go"), []byte("package untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyCheckout(dir, head); err == nil {
		t.Fatal("untracked file accepted")
	}
}

func TestPrepareCheckoutReusesCheckoutAfterRemovingGeneratedTests(t *testing.T) {
	root := t.TempDir()
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, paths.checkout, "init")
	runGitTest(t, paths.checkout, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	head := strings.TrimSpace(runGitOutput(t, paths.checkout, "rev-parse", "HEAD"))

	generated := filepath.Join(paths.checkout, "integration", "censorshipplugin")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "generated_test.go"), []byte("package censorshipplugin\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := prepareCheckout(paths, head); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(generated); !os.IsNotExist(err) {
		t.Fatalf("generated integration tests remain after checkout preparation: %v", err)
	}
}

func TestPrepareModelMapperCheckoutReusesCleanCheckout(t *testing.T) {
	root := t.TempDir()
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.mapperCheckout, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, paths.mapperCheckout, "init")
	runGitTest(t, paths.mapperCheckout, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	head := strings.TrimSpace(runGitOutput(t, paths.mapperCheckout, "rev-parse", "HEAD"))

	if err := prepareModelMapperCheckout(paths, head); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runGitOutput(t, paths.mapperCheckout, "rev-parse", "HEAD")); got != head {
		t.Fatalf("mapper checkout HEAD = %q, want %q", got, head)
	}
}

func TestPreparePluginPlatformDirRemovesStaleArtifacts(t *testing.T) {
	root := t.TempDir()
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	platformDir := pluginPlatformDir(paths)
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(platformDir, "stale.dll"), []byte("stale native library"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := preparePluginPlatformDir(paths)
	if err != nil {
		t.Fatal(err)
	}
	if got != platformDir {
		t.Fatalf("plugin platform directory = %q, want %q", got, platformDir)
	}
	entries, err := os.ReadDir(platformDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("plugin platform directory contains %d stale artifacts", len(entries))
	}
}

func TestPrepareCheckoutRejectsSymlinkedCheckoutBeforeRemovingGeneratedTests(t *testing.T) {
	root := t.TempDir()
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	runGitTest(t, outside, "init")
	runGitTest(t, outside, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "fixture")
	head := strings.TrimSpace(runGitOutput(t, outside, "rev-parse", "HEAD"))
	generated := filepath.Join(outside, "integration", "censorshipplugin", "generated_test.go")
	if err := os.MkdirAll(filepath.Dir(generated), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generated, []byte("package censorshipplugin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.integrationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	linkDirectory(t, outside, paths.checkout)

	if err := prepareCheckout(paths, head); err == nil {
		t.Fatal("symlinked checkout accepted")
	}
	if _, err := os.Stat(generated); err != nil {
		t.Fatalf("generated test outside integration root was removed: %v", err)
	}
}

func linkDirectory(t *testing.T, target, link string) {
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

func TestCopyIntegrationFilesCopiesBenchmarkFixture(t *testing.T) {
	root := t.TempDir()
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	paths.repositoryRoot = root

	integrationDir := filepath.Join(root, "integration")
	if err := os.MkdirAll(integrationDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(integrationDir, "doc.go"), []byte("package censorshipintegration\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(root, ".github", "scripts", "testdata", "abi_benchmark_test.go")
	if err := os.MkdirAll(filepath.Dir(fixture), 0o755); err != nil {
		t.Fatal(err)
	}
	const want = "package censorshipintegration\n\nfunc BenchmarkDynamicABIRequestInterceptors() {}\n"
	if err := os.WriteFile(fixture, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyIntegrationFiles(paths); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(paths.checkout, "integration", "censorshipplugin", "abi_benchmark_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("copied fixture = %q, want %q", got, want)
	}
}

func TestBenchmarkPlacementRunsOnlyFixtureInCPAIntegrationPackage(t *testing.T) {
	got := integrationTestArgs(true)
	want := []string{
		"test", "-tags=integration", "-count=1", "-v", "./integration/censorshipplugin",
		"-run", "^$", "-bench", "^BenchmarkDynamicABIRequestInterceptors$", "-benchmem",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("benchmark arguments = %q, want %q", got, want)
	}
}

func TestParseBenchmarkMode(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
		err  bool
	}{
		{args: nil, want: false},
		{args: []string{"-bench-abi"}, want: true},
		{args: []string{"-unexpected"}, err: true},
	} {
		got, err := parseBenchmarkMode(tc.args)
		if (err != nil) != tc.err || got != tc.want {
			t.Errorf("parseBenchmarkMode(%q) = %t, %v; want %t, error %t", tc.args, got, err, tc.want, tc.err)
		}
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
