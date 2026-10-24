package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestStagePluginLibraryReplacesPlatformDirectory(t *testing.T) {
	root := t.TempDir()
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(pluginPlatformDir(paths), "stale-plugin")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale native library"), 0o644); err != nil {
		t.Fatal(err)
	}
	extension, err := pluginExtension(runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "prebuilt"+extension)
	sourceBytes := []byte{0, 1, 2, 255, 3}
	if err := os.WriteFile(source, sourceBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	pluginDir, err := stagePluginLibrary(paths, source)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(paths.run, "plugins"); pluginDir != want {
		t.Fatalf("plugin directory = %q, want %q", pluginDir, want)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale plugin remains after staging: %v", err)
	}
	staged, err := os.ReadFile(filepath.Join(paths.run, "plugins", runtime.GOOS, runtime.GOARCH, "censorship"+extension))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(staged, sourceBytes) {
		t.Fatalf("staged library = %v, want %v", staged, sourceBytes)
	}
	unchanged, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchanged, sourceBytes) {
		t.Fatalf("source library = %v, want %v", unchanged, sourceBytes)
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

func TestIntegrationTestArgsSelectsRunnerMode(t *testing.T) {
	base := []string{"test", "-tags=integration", "-count=1", "-v", "./integration/censorshipplugin"}
	for _, tc := range []struct {
		name    string
		options runnerOptions
		want    []string
	}{
		{name: "full", want: base},
		{
			name:    "benchmark",
			options: runnerOptions{benchmark: true},
			want:    append(append([]string{}, base...), "-run", "^$", "-bench", "^BenchmarkDynamicABIRequestInterceptors$", "-benchmem"),
		},
		{
			name:    "ABI smoke",
			options: runnerOptions{abiSmokeLibrary: "censorship.dll"},
			want:    append(append([]string{}, base...), "-run", "^TestDynamicABIActiveAfter$"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := integrationTestArgs(tc.options); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("integration test arguments = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseRunnerOptionsSelectsExactlyOneMode(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		benchEnv string
		want     runnerOptions
		err      bool
	}{
		{name: "full", want: runnerOptions{}},
		{name: "benchmark argument", args: []string{"-bench-abi"}, want: runnerOptions{benchmark: true}},
		{name: "ABI smoke argument", args: []string{"-abi-smoke", "censorship.dll"}, want: runnerOptions{abiSmokeLibrary: "censorship.dll"}},
		{name: "benchmark environment", benchEnv: "1", want: runnerOptions{benchmark: true}},
		{name: "ABI smoke overrides benchmark environment", args: []string{"-abi-smoke", "censorship.dll"}, benchEnv: "1", want: runnerOptions{abiSmokeLibrary: "censorship.dll"}},
		{name: "missing ABI smoke library", args: []string{"-abi-smoke"}, err: true},
		{name: "empty ABI smoke library", args: []string{"-abi-smoke", ""}, err: true},
		{name: "mixed CLI modes", args: []string{"-bench-abi", "-abi-smoke", "censorship.dll"}, err: true},
		{name: "unknown argument", args: []string{"-unexpected"}, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRunnerOptions(tc.args, tc.benchEnv)
			if (err != nil) != tc.err || got != tc.want {
				t.Fatalf("parseRunnerOptions(%q, %q) = %#v, %v; want %#v, error %t", tc.args, tc.benchEnv, got, err, tc.want, tc.err)
			}
		})
	}
}

func TestBuildCPAUsesGoCommandTextAndCheckoutDirectory(t *testing.T) {
	t.Setenv("GO", "go -C .")
	t.Setenv("GOENV", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	paths, err := resolveRunnerPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeRunnerFixture(t, filepath.Join(paths.repositoryRoot, "go.mod"), "module example.com/plugin-fixture\n\ngo 1.24\n")
	writeRunnerFixture(t, filepath.Join(paths.repositoryRoot, "main.go"), "this plugin root must not compile\n")
	writeRunnerFixture(t, filepath.Join(paths.checkout, "go.mod"), "module example.com/cpa-fixture\n\ngo 1.24\n")
	writeRunnerFixture(t, filepath.Join(paths.checkout, "cmd", "server", "main.go"), `package main
import "fmt"
func main() { fmt.Println("CPA checkout fixture") }
`)
	t.Logf("GO=%q cwd=%q argv=%q", goCommand(), paths.checkout, []string{"build", "-trimpath", "-o", paths.bin, "./cmd/server"})
	if err := buildCPA(paths); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(paths.bin).CombinedOutput()
	if err != nil || string(output) != "CPA checkout fixture\n" {
		t.Fatalf("CPA fixture output = %q, error = %v", output, err)
	}
}

func TestFullIntegrationTimeoutUsesEffectiveGoFlags(t *testing.T) {
	isolateRunnerGo(t)
	t.Setenv("GO", "")
	paths, report := newRunnerIntegrationFixture(t)
	if err := runIntegrationTests(paths, paths.run, runnerOptions{}); err != nil {
		t.Fatal(err)
	}
	got := readRunnerProbe(t, report)
	t.Logf("GO=%q cwd=%q actual timeout=%q GOFLAGS=%q GOENV=%q", goCommand(), paths.checkout, got.Timeout, os.Getenv("GOFLAGS"), os.Getenv("GOENV"))
	if got.Timeout != "30m0s" {
		t.Fatalf("full child timeout = %q, want 30m0s", got.Timeout)
	}
}

func TestFullIntegrationGoFlagsOverrides(t *testing.T) {
	isolateRunnerGo(t)
	t.Setenv("GO", "go -C .")
	paths, report := newRunnerIntegrationFixture(t)
	for _, tc := range []struct {
		name, flags, timeout, skip string
	}{
		{"shorter", "-timeout=2m", "2m0s", ""},
		{"longer", "-timeout=2h", "2h0m0s", ""},
		{"disabled", "-timeout=0", "0s", ""},
		{"test spelling", "-test.timeout=3m", "3m0s", ""},
		{"duplicate last wins", "-timeout=2m -test.timeout=3m -timeout=4m", "4m0s", ""},
		{"quotes and spaces", `-timeout=2m '-skip=no such test' "-count=5"`, "2m0s", "no such test"},
	} {
		for _, persistent := range []bool{false, true} {
			name := tc.name + "/OS"
			if persistent {
				name = tc.name + "/GOENV"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("GOFLAGS", "")
				if persistent {
					t.Setenv("GOENV", filepath.Join(t.TempDir(), "go-env"))
					if err := runGoCommand(paths.checkout, nil, "env", "-w", "GOFLAGS="+tc.flags); err != nil {
						t.Fatal(err)
					}
				} else {
					t.Setenv("GOFLAGS", tc.flags)
				}
				if err := runIntegrationTests(paths, paths.run, runnerOptions{}); err != nil {
					t.Fatal(err)
				}
				got := readRunnerProbe(t, report)
				t.Logf("source=%s actual=%#v", name, got)
				if got.Timeout != tc.timeout || got.Skip != tc.skip || got.Flags != "-timeout=30m "+tc.flags || got.Count != "1" || got.Verbose != "true" || got.Bin != paths.bin || got.PluginDir != paths.run || got.GOENV != os.Getenv("GOENV") {
					t.Fatalf("full child received %#v", got)
				}
				if got := os.Getenv("GOFLAGS"); (!persistent && got != tc.flags) || (persistent && got != "") {
					t.Fatalf("parent GOFLAGS changed: %q", got)
				}
			})
		}
	}
	t.Run("OS overrides persisted flags", func(t *testing.T) {
		t.Setenv("GOENV", filepath.Join(t.TempDir(), "go-env"))
		if err := runGoCommand(paths.checkout, nil, "env", "-w", "GOFLAGS=-timeout=7m"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GOFLAGS", "-timeout=1m")
		if err := runIntegrationTests(paths, paths.run, runnerOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := readRunnerProbe(t, report); got.Timeout != "1m0s" || got.Flags != "-timeout=30m -timeout=1m" {
			t.Fatalf("OS override received %#v", got)
		}
	})
}

func TestFullIntegrationGoFlagsPreserveWrapperOverrides(t *testing.T) {
	isolateRunnerGo(t)
	paths, report := newRunnerIntegrationFixture(t)
	wrapper, calls := newRunnerGoWrapper(t, paths.repositoryRoot)
	t.Setenv("GO", shellQuoteRunnerTest(wrapper)+" -C .")
	for _, tc := range []struct {
		name, action, osFlags, wrapperFlags, persisted, timeout, effective string
	}{
		{"passthrough default", "", "", "", "", "30m0s", ""},
		{"wrapper shorter", "set", "", "-timeout=2m", "", "2m0s", "-timeout=2m"},
		{"wrapper longer", "set", "", "-timeout=2h", "", "2h0m0s", "-timeout=2h"},
		{"wrapper disabled", "set", "", "-timeout=0", "", "0s", "-timeout=0"},
		{"wrapper test spelling", "set", "", "-test.timeout=3m", "", "3m0s", "-test.timeout=3m"},
		{"wrapper duplicate", "set", "", "-timeout=2m -test.timeout=4m", "", "4m0s", "-timeout=2m -test.timeout=4m"},
		{"wrapper quotes", "set", "", `-timeout=2m '-skip=no such test'`, "", "2m0s", `-timeout=2m '-skip=no such test'`},
		{"wrapper rewrites without timeout", "set", "-timeout=1m", "-trimpath", "", "10m0s", "-trimpath"},
		{"wrapper clears OS flags", "clear", "-timeout=1m", "", "", "10m0s", ""},
		{"wrapper empty reads GOENV", "empty", "-timeout=1m", "", "-timeout=6m", "6m0s", "-timeout=6m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOFLAGS", "")
			if tc.persisted != "" {
				t.Setenv("GOENV", filepath.Join(t.TempDir(), "go-env"))
				cmd := exec.Command(os.Getenv("CPA_RUNNER_DRIVER"), "env", "-w", "GOFLAGS="+tc.persisted)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("persist isolated GOFLAGS: %v\n%s", err, out)
				}
			}
			t.Setenv("GOFLAGS", tc.osFlags)
			t.Setenv("CPA_RUNNER_WRAPPER_ACTION", tc.action)
			t.Setenv("CPA_RUNNER_WRAPPER_FLAGS", tc.wrapperFlags)
			writeRunnerFixture(t, calls, "")
			if err := runIntegrationTests(paths, paths.run, runnerOptions{}); err != nil {
				t.Fatal(err)
			}
			got := readRunnerProbe(t, report)
			data, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("actual wrapper calls=%s child=%#v", data, got)
			var trace []runnerGoCall
			decoder := json.NewDecoder(bytes.NewReader(data))
			for {
				var call runnerGoCall
				if err := decoder.Decode(&call); err == io.EOF {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				trace = append(trace, call)
			}
			if len(trace) != 2 || !reflect.DeepEqual(trace[0].Args, []string{"-C", ".", "env", "-json", "GOFLAGS"}) || !reflect.DeepEqual(trace[1].Args, []string{"-C", ".", "test", "-tags=integration", "-count=1", "-v", "./integration/censorshipplugin"}) {
				t.Fatalf("unexpected query/test calls: %#v", trace)
			}
			for _, call := range trace {
				if call.CWD != paths.checkout || call.Bin != paths.bin || call.PluginDir != paths.run || call.Exit != 0 {
					t.Fatalf("wrapper call changed cwd/env/exit: %#v", call)
				}
			}
			var query struct{ GOFLAGS string }
			if err := json.Unmarshal([]byte(trace[0].Output), &query); err != nil {
				t.Fatal(err)
			}
			if query.GOFLAGS != tc.effective || trace[1].IncomingFlags != "-timeout=30m "+tc.effective || got.Timeout != tc.timeout || got.Flags != trace[1].Flags {
				t.Fatalf("wrapper timeout/flags changed: trace=%#v child=%#v", trace, got)
			}
		})
	}
}

func TestIntegrationModesKeepExistingGoFlagsAndTimeout(t *testing.T) {
	isolateRunnerGo(t)
	t.Setenv("GO", "go -C .")
	paths, report := newRunnerIntegrationFixture(t)
	for _, flags := range []string{"", "-timeout=2m"} {
		for _, tc := range []struct {
			name  string
			args  []string
			bench string
		}{
			{"smoke", []string{"-abi-smoke", "fixture.dll"}, ""},
			{"benchmark argument", []string{"-bench-abi"}, ""},
			{"BENCH environment", nil, "1"},
			{"smoke precedes BENCH", []string{"-abi-smoke", "fixture.dll"}, "1"},
		} {
			t.Run(tc.name+"/"+flags, func(t *testing.T) {
				t.Setenv("GOFLAGS", flags)
				t.Setenv("BENCH", tc.bench)
				options, err := parseRunnerOptions(tc.args, os.Getenv("BENCH"))
				if err != nil {
					t.Fatal(err)
				}
				if err := runIntegrationTests(paths, paths.run, options); err != nil {
					t.Fatal(err)
				}
				got := readRunnerProbe(t, report)
				t.Logf("mode=%s options=%#v actual=%#v", tc.name, options, got)
				wantTimeout := "10m0s"
				if flags != "" {
					wantTimeout = "2m0s"
				}
				if got.Timeout != wantTimeout || got.Flags != flags || got.Count != "1" || got.Verbose != "true" {
					t.Fatalf("non-full child received %#v", got)
				}
			})
		}
	}
}

func TestFullIntegrationPropagatesQueryAndTestFailures(t *testing.T) {
	isolateRunnerGo(t)
	paths, report := newRunnerIntegrationFixture(t)
	for _, tc := range []struct {
		name, command, flags, fail, delay, message string
	}{
		{"query failure", "go -C missing-runner-directory", "", "", "", "query effective GOFLAGS"},
		{"invalid query JSON", "printf invalid", "", "", "", "decode effective GOFLAGS"},
		{"invalid flags", "go -C .", "-not-a-runner-flag", "", "", "run go"},
		{"malformed quoted flags", "go", "'-timeout=1m", "", "", "run go"},
		{"child failure", "go -C .", "", "1", "", "run go"},
		{"short timeout terminates child", "go -C .", "-timeout=100ms", "", "1", "run go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GO", tc.command)
			t.Setenv("GOFLAGS", tc.flags)
			t.Setenv("CPA_RUNNER_FAIL", tc.fail)
			t.Setenv("CPA_RUNNER_DELAY", tc.delay)
			start := time.Now()
			err := runIntegrationTests(paths, paths.run, runnerOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("full integration error = %v, want %q", err, tc.message)
			}
			t.Logf("GO=%q flags=%q error=%v elapsed=%s", tc.command, tc.flags, err, time.Since(start))
			if tc.delay != "" {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 || time.Since(start) >= 30*time.Second {
					t.Fatalf("short timeout did not terminate the child: %v", err)
				}
				if got := readRunnerProbe(t, report); got.Timeout != "100ms" {
					t.Fatalf("terminated child received %#v", got)
				}
			}
		})
	}
}

func TestGoCommandTextPreservesCWDArgsAndEnvironment(t *testing.T) {
	isolateRunnerGo(t)
	goExe, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	probe := writeRunnerProbe(t, root)
	spacedGo := filepath.Join(root, "Go executable", filepath.Base(goExe))
	contents, err := os.ReadFile(goExe)
	if err != nil {
		t.Fatal(err)
	}
	writeRunnerFixture(t, spacedGo, string(contents))
	if err := os.Chmod(spacedGo, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOROOT", runtime.GOROOT())
	args := []string{"", "two words", "single'quote", `double"quote`, `C:\path with space\`, `a'\"b\c`, "$HOME", "a;exit 5"}
	for _, tc := range []struct {
		name, command, marker string
	}{
		{"default", "", "extra"},
		{"absolute", goExe, "extra"},
		{"literal spaced executable", spacedGo, "extra"},
		{"quoted executable", shellQuoteRunnerTest(goExe), "extra"},
		{"quoted spaced executable", shellQuoteRunnerTest(spacedGo), "extra"},
		{"directory", "go -C .", "extra"},
		{"quoted absolute directory", shellQuoteRunnerTest(goExe) + " -C .", "extra"},
		{"env wrapper", "env -u CPA_RUNNER_MARKER " + shellQuoteRunnerTest(goExe) + " -C .", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GO", tc.command)
			for _, cwd := range []string{"cpa", "plugin", "model-mapper"} {
				dir := filepath.Join(root, cwd)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				report := filepath.Join(root, "report.json")
				env := []string{"CPA_RUNNER_REPORT=" + report, "CPA_RUNNER_MARKER=extra"}
				if err := runGoCommand(dir, env, append([]string{"run", probe}, args...)...); err != nil {
					t.Fatal(err)
				}
				got := readRunnerProbe(t, report)
				t.Logf("GO=%q cwd=%q args=%q marker=%q", tc.command, got.CWD, got.Args, got.Marker)
				if filepath.Clean(got.CWD) != dir || !reflect.DeepEqual(got.Args, args) || got.Marker != tc.marker || got.GO != tc.command {
					t.Fatalf("received %#v, want cwd=%q args=%q marker=%q GO=%q", got, dir, args, tc.marker, tc.command)
				}
			}
		})
	}
}

func TestGoCommandDirectoryIsRelativeToEachCaller(t *testing.T) {
	isolateRunnerGo(t)
	t.Setenv("GO", "go -C relative")
	root := t.TempDir()
	probe := writeRunnerProbe(t, root)
	for _, caller := range []string{"cpa", "plugin", "model-mapper"} {
		dir := filepath.Join(root, caller)
		want := filepath.Join(dir, "relative")
		if err := os.MkdirAll(want, 0o755); err != nil {
			t.Fatal(err)
		}
		report := filepath.Join(root, "report.json")
		if err := runGoCommand(dir, []string{"CPA_RUNNER_REPORT=" + report}, "run", probe); err != nil {
			t.Fatal(err)
		}
		got := readRunnerProbe(t, report)
		t.Logf("GO=%q caller cwd=%q actual cwd=%q", goCommand(), dir, got.CWD)
		if filepath.Clean(got.CWD) != want {
			t.Fatalf("-C cwd = %q, want %q", got.CWD, want)
		}
	}
}

func TestGoCommandLiteralExecutableWithoutShell(t *testing.T) {
	isolateRunnerGo(t)
	goExe, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("sh"); err == nil {
		t.Fatal("shell is still available")
	}
	t.Setenv("GO", goExe)
	if err := runGoCommand(t.TempDir(), nil, "version"); err != nil {
		t.Fatalf("literal executable without shell: %v", err)
	}
	t.Setenv("GO", shellQuoteRunnerTest(goExe)+" -C .")
	err = runGoCommand(t.TempDir(), nil, "version")
	var launchErr *exec.Error
	if !errors.As(err, &launchErr) || launchErr.Name != "sh" {
		t.Fatalf("complex GO without shell error = %v, want sh startup failure", err)
	}
	t.Logf("literal executable succeeded without shell; complex GO returned %v", err)
}

func TestGoCommandTextPropagatesFailure(t *testing.T) {
	isolateRunnerGo(t)
	for _, tc := range []struct {
		name, command string
		args          []string
		exit          int
	}{
		{"unknown command", "go -C .", []string{"not-a-go-command"}, 2},
		{"missing directory", "go -C missing-runner-directory", []string{"version"}, 1},
		{"missing executable", "missing-runner-go-executable", []string{"version"}, 127},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GO", tc.command)
			err := runGoCommand(t.TempDir(), nil, tc.args...)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != tc.exit {
				t.Fatalf("GO=%q args=%q: %v, want exit %d", tc.command, tc.args, err, tc.exit)
			}
			t.Logf("GO=%q args=%q exit=%d", tc.command, tc.args, exitErr.ExitCode())
		})
	}
}

func TestMakePreservesGoCommandText(t *testing.T) {
	isolateRunnerGo(t)
	// 本 fixture 分别检查三种 GO 来源，不继承外层 make 的命令行覆盖。
	t.Setenv("MAKEFLAGS", "")
	t.Setenv("MFLAGS", "")
	t.Setenv("MAKEOVERRIDES", "")
	repo, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	makefile := filepath.ToSlash(filepath.Join(repo, "Makefile"))
	root := t.TempDir()
	probe := writeRunnerProbe(t, root)
	report := filepath.Join(root, "report.json")
	t.Setenv("CPA_RUNNER_REPORT", report)
	fixture := filepath.Join(root, "probe.mk")
	writeRunnerFixture(t, fixture, "runner-go-probe:\n\t$(GO) run "+shellQuoteRunnerTest(probe)+" 'two words' ''\n")
	for _, tc := range []struct {
		name, command, origin string
	}{
		{"default", "", "default"},
		{"environment", "go -C .", "environment"},
		{"command line", "env -u CPA_RUNNER_MARKER go -C .", "command line"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GO", "")
			if err := os.Unsetenv("GO"); err != nil {
				t.Fatal(err)
			}
			args := []string{"-f", makefile, "-f", fixture, "runner-go-probe"}
			if tc.origin == "environment" {
				t.Setenv("GO", tc.command)
			} else if tc.origin == "command line" {
				args = append(args, "GO="+tc.command)
			}
			cmd := exec.Command("make", args...)
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("make %q: %v\n%s", args, err, output)
			}
			got := readRunnerProbe(t, report)
			t.Logf("make %q output=%s received=%#v", args, output, got)
			if got.GO != tc.command || filepath.Clean(got.CWD) != root || !reflect.DeepEqual(got.Args, []string{"two words", ""}) {
				t.Fatalf("Make received %#v", got)
			}
		})
	}
	cmd := exec.Command("make", "-f", makefile, "integration", "GO=")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Make explicit empty GO succeeded: %s", output)
	}
	t.Logf("make integration GO= retains failure: %v\n%s", err, output)
}

type runnerProbe struct {
	Args                                 []string
	CWD, Marker, GO                      string
	Timeout, Flags, Skip, Count, Verbose string
	Bin, PluginDir, GOENV                string
}

func newRunnerIntegrationFixture(t *testing.T) (runnerPaths, string) {
	t.Helper()
	paths, err := resolveRunnerPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeRunnerFixture(t, filepath.Join(paths.checkout, "go.mod"), "module example.com/cpa-fixture\n\ngo 1.24\n")
	writeRunnerFixture(t, filepath.Join(paths.checkout, "integration", "censorshipplugin", "timeout_test.go"), `//go:build integration

package censorshipplugin
import ("encoding/json"; "flag"; "os"; "testing"; "time")
func recordRunner(t testing.TB) {
    t.Helper()
    cwd, err := os.Getwd(); if err != nil { t.Fatal(err) }
    data, err := json.Marshal(struct {
        Args []string
        CWD, Timeout, Flags, Skip, Count, Verbose, Bin, PluginDir, GOENV string
    }{os.Args[1:], cwd, flag.Lookup("test.timeout").Value.String(), os.Getenv("GOFLAGS"),
      flag.Lookup("test.skip").Value.String(), flag.Lookup("test.count").Value.String(),
      flag.Lookup("test.v").Value.String(), os.Getenv("CPA_INTEGRATION_BIN"),
      os.Getenv("CENSORSHIP_PLUGIN_DIR"), os.Getenv("GOENV")})
    if err != nil { t.Fatal(err) }
    if err := os.WriteFile(os.Getenv("CPA_RUNNER_REPORT"), data, 0600); err != nil { t.Fatal(err) }
    if os.Getenv("CPA_RUNNER_DELAY") == "1" { time.Sleep(time.Minute) }
    if os.Getenv("CPA_RUNNER_FAIL") == "1" { t.Fatal("requested child failure") }
}
func TestDynamicABIActiveAfter(t *testing.T) { recordRunner(t) }
func BenchmarkDynamicABIRequestInterceptors(b *testing.B) { recordRunner(b) }
`)
	report := filepath.Join(paths.repositoryRoot, "report.json")
	t.Setenv("CPA_RUNNER_REPORT", report)
	return paths, report
}

type runnerGoCall struct {
	Args                                              []string
	CWD, IncomingFlags, Flags, Bin, PluginDir, Output string
	Exit                                              int
}

func newRunnerGoWrapper(t *testing.T, root string) (string, string) {
	t.Helper()
	goExe, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CPA_RUNNER_DRIVER", goExe)
	calls := filepath.Join(root, "go-calls.jsonl")
	t.Setenv("CPA_RUNNER_CALLS", calls)
	source := filepath.Join(root, "wrapper.go")
	writeRunnerFixture(t, source, `package main
import ("bytes"; "encoding/json"; "errors"; "io"; "os"; "os/exec")
func main() {
    incoming := os.Getenv("GOFLAGS")
    switch os.Getenv("CPA_RUNNER_WRAPPER_ACTION") {
    case "set": if err := os.Setenv("GOFLAGS", os.Getenv("CPA_RUNNER_WRAPPER_FLAGS")); err != nil { panic(err) }
    case "clear": if err := os.Unsetenv("GOFLAGS"); err != nil { panic(err) }
    case "empty": if err := os.Setenv("GOFLAGS", ""); err != nil { panic(err) }
    }
    cwd, err := os.Getwd(); if err != nil { panic(err) }
    var output bytes.Buffer
    cmd := exec.Command(os.Getenv("CPA_RUNNER_DRIVER"), os.Args[1:]...)
    cmd.Stdout = io.MultiWriter(os.Stdout, &output); cmd.Stderr = os.Stderr
    code := 0
    if err := cmd.Run(); err != nil {
        var exitErr *exec.ExitError
        if !errors.As(err, &exitErr) { panic(err) }
        code = exitErr.ExitCode()
    }
    f, err := os.OpenFile(os.Getenv("CPA_RUNNER_CALLS"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
    if err != nil { panic(err) }
    err = json.NewEncoder(f).Encode(struct {
        Args []string
        CWD, IncomingFlags, Flags, Bin, PluginDir, Output string
        Exit int
    }{os.Args[1:], cwd, incoming, os.Getenv("GOFLAGS"), os.Getenv("CPA_INTEGRATION_BIN"),
      os.Getenv("CENSORSHIP_PLUGIN_DIR"), output.String(), code})
    if err != nil { panic(err) }
    if err := f.Close(); err != nil { panic(err) }
    os.Exit(code)
}
`)
	wrapper := filepath.Join(root, "go-wrapper")
	if runtime.GOOS == "windows" {
		wrapper += ".exe"
	}
	cmd := exec.Command(goExe, "build", "-o", wrapper, source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build real Go forwarding wrapper: %v\n%s", err, out)
	}
	return wrapper, calls
}

func writeRunnerProbe(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "probe.go")
	writeRunnerFixture(t, path, `package main
import ("encoding/json"; "os")
func main() {
    cwd, err := os.Getwd(); if err != nil { panic(err) }
    f, err := os.Create(os.Getenv("CPA_RUNNER_REPORT")); if err != nil { panic(err) }
    defer f.Close()
    if err := json.NewEncoder(f).Encode(struct { Args []string; CWD, Marker, GO string }{
        os.Args[1:], cwd, os.Getenv("CPA_RUNNER_MARKER"), os.Getenv("GO"),
    }); err != nil { panic(err) }
}
`)
	return path
}

func readRunnerProbe(t *testing.T, path string) runnerProbe {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result runnerProbe
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func isolateRunnerGo(t *testing.T) {
	t.Helper()
	t.Setenv("GOENV", "off")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOOS", runtime.GOOS)
	t.Setenv("GOARCH", runtime.GOARCH)
}

func shellQuoteRunnerTest(value string) string {
	return "'" + strings.ReplaceAll(filepath.ToSlash(value), "'", "'\"'\"'") + "'"
}

func writeRunnerFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
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
