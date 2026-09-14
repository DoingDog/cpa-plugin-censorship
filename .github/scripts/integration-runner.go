package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	cpaSHA            = "c76dfd4e0edabab9000628b1560ab8ab379eadb8"
	cpaRemote         = "https://github.com/router-for-me/CLIProxyAPI"
	modelMapperSHA    = "8fe4839dd2c39a4b0537447c4ac35a9f1d699bbf"
	modelMapperRemote = "https://github.com/DoingDog/cpa-plugin-model-mapper"
)

type runnerPaths struct {
	repositoryRoot  string
	integrationRoot string
	checkout        string
	mapperCheckout  string
	bin             string
	run             string
}

type runnerOptions struct {
	benchmark       bool
	abiSmokeLibrary string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func run() error {
	options, err := parseRunnerOptions(os.Args[1:], os.Getenv("BENCH"))
	if err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		return err
	}
	if err := prepareCheckout(paths, cpaSHA); err != nil {
		return err
	}
	if options.abiSmokeLibrary != "" {
		if err := buildCPA(paths); err != nil {
			return err
		}
		pluginDir, err := stagePluginLibrary(paths, options.abiSmokeLibrary)
		if err != nil {
			return err
		}
		if err := copyIntegrationFiles(paths); err != nil {
			return err
		}
		return runIntegrationTests(paths, pluginDir, options)
	}
	if err := prepareModelMapperCheckout(paths, modelMapperSHA); err != nil {
		return err
	}
	if err := buildCPA(paths); err != nil {
		return err
	}
	platformDir, err := preparePluginPlatformDir(paths)
	if err != nil {
		return err
	}
	pluginDir, err := buildPlugin(paths, platformDir)
	if err != nil {
		return err
	}
	if err := buildModelMapper(paths, platformDir); err != nil {
		return err
	}
	if err := verifyCheckout(paths.mapperCheckout, modelMapperSHA); err != nil {
		return err
	}
	if err := copyIntegrationFiles(paths); err != nil {
		return err
	}
	return runIntegrationTests(paths, pluginDir, options)
}

func parseRunnerOptions(args []string, benchEnv string) (runnerOptions, error) {
	switch {
	case len(args) == 0:
		return runnerOptions{benchmark: benchEnv == "1"}, nil
	case len(args) == 1 && args[0] == "-bench-abi":
		return runnerOptions{benchmark: true}, nil
	case len(args) == 2 && args[0] == "-abi-smoke" && args[1] != "":
		return runnerOptions{abiSmokeLibrary: args[1]}, nil
	default:
		return runnerOptions{}, fmt.Errorf("usage: integration-runner [-bench-abi | -abi-smoke <library>]")
	}
}

func repositoryRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", fmt.Errorf("resolve repository root: git returned an empty path")
	}
	return filepath.Abs(root)
}

func resolveRunnerPaths(root string) (runnerPaths, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return runnerPaths{}, fmt.Errorf("resolve repository root %q: %w", root, err)
	}
	integrationRoot := filepath.Join(root, ".integration")
	bin := filepath.Join(integrationRoot, "bin")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	paths := runnerPaths{
		repositoryRoot:  root,
		integrationRoot: integrationRoot,
		checkout:        filepath.Join(integrationRoot, "cpa"),
		mapperCheckout:  filepath.Join(integrationRoot, "model-mapper"),
		bin:             bin,
		run:             filepath.Join(integrationRoot, "run"),
	}
	for _, path := range []string{paths.checkout, paths.mapperCheckout, paths.bin, paths.run} {
		if err := requireContained(integrationRoot, path); err != nil {
			return runnerPaths{}, err
		}
	}
	return paths, nil
}

func requireContained(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("resolve %q relative to %q: %w", path, root, err)
	}
	if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes integration root %q", path, root)
	}
	for ancestor := path; ; ancestor = filepath.Dir(ancestor) {
		info, err := os.Lstat(ancestor)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect path %q: %w", ancestor, err)
		}
		if err == nil && info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return fmt.Errorf("path %q escapes integration root %q through a link", path, root)
		}
		if ancestor == root {
			break
		}
	}
	canonicalRoot, err := canonicalExistingPath(root)
	if err != nil {
		return err
	}
	canonicalPath, err := canonicalExistingPath(path)
	if err != nil {
		return err
	}
	canonicalRel, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil {
		return fmt.Errorf("resolve %q relative to %q: %w", canonicalPath, canonicalRoot, err)
	}
	if canonicalRel == "." || canonicalRel == ".." || filepath.IsAbs(canonicalRel) || strings.HasPrefix(canonicalRel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes integration root %q", path, root)
	}
	return nil
}

func canonicalExistingPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make path absolute %q: %w", path, err)
	}
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for _, component := range missing {
				resolved = filepath.Join(resolved, component)
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve path %q: %w", path, err)
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("resolve path %q: no existing ancestor", path)
		}
		missing = append([]string{filepath.Base(path)}, missing...)
		path = parent
	}
}

func removeContained(root, path string) error {
	if err := requireContained(root, path); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s: %w", filepath.ToSlash(path), err)
	}
	return nil
}

func pluginExtension(goos string) (string, error) {
	switch goos {
	case "windows":
		return ".dll", nil
	case "darwin":
		return ".dylib", nil
	case "linux", "freebsd":
		return ".so", nil
	default:
		return "", fmt.Errorf("unsupported GOOS %q", goos)
	}
}

func verifyCheckout(path, wantSHA string) error {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("verify checkout %s: %w", filepath.ToSlash(path), err)
	}
	got := strings.TrimSpace(string(output))
	if got != wantSHA {
		return fmt.Errorf("checkout HEAD is %q, want %q", got, wantSHA)
	}
	cmd = exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = path
	cmd.Stderr = os.Stderr
	output, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("inspect checkout %s: %w", filepath.ToSlash(path), err)
	}
	if status := strings.TrimSpace(string(output)); status != "" {
		return fmt.Errorf("checkout is dirty: %s", status)
	}
	return nil
}

func prepareCheckout(paths runnerPaths, wantSHA string) error {
	generatedTests := filepath.Join(paths.checkout, "integration", "censorshipplugin")
	if err := removeContained(paths.integrationRoot, generatedTests); err != nil {
		return err
	}
	return preparePinnedCheckout(paths.integrationRoot, paths.checkout, cpaRemote, wantSHA)
}

func prepareModelMapperCheckout(paths runnerPaths, wantSHA string) error {
	return preparePinnedCheckout(paths.integrationRoot, paths.mapperCheckout, modelMapperRemote, wantSHA)
}

func preparePinnedCheckout(integrationRoot, checkout, remote, revision string) error {
	if err := requireContained(integrationRoot, checkout); err != nil {
		return err
	}
	if err := verifyCheckout(checkout, revision); err == nil {
		return nil
	}
	if err := removeContained(integrationRoot, checkout); err != nil {
		return err
	}
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		return fmt.Errorf("create checkout directory: %w", err)
	}
	commands := [][]string{
		{"git", "init"},
		{"git", "remote", "add", "origin", remote},
		{"git", "fetch", "--depth=1", "origin", revision},
		{"git", "checkout", "--detach", "FETCH_HEAD"},
	}
	for _, command := range commands {
		if err := runCommand(checkout, nil, command[0], command[1:]...); err != nil {
			return err
		}
	}
	return verifyCheckout(checkout, revision)
}

func buildCPA(paths runnerPaths) error {
	if err := removeContained(paths.integrationRoot, paths.bin); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paths.bin), 0o755); err != nil {
		return fmt.Errorf("create CPA binary directory: %w", err)
	}
	return runCommand(paths.checkout, nil, goCommand(), "build", "-trimpath", "-o", paths.bin, "./cmd/server")
}

func pluginPlatformDir(paths runnerPaths) string {
	return filepath.Join(paths.run, "plugins", runtime.GOOS, runtime.GOARCH)
}

func preparePluginPlatformDir(paths runnerPaths) (string, error) {
	platformDir := pluginPlatformDir(paths)
	if err := requireContained(paths.integrationRoot, platformDir); err != nil {
		return "", err
	}
	if err := removeContained(paths.integrationRoot, platformDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		return "", fmt.Errorf("create plugin directory: %w", err)
	}
	return platformDir, nil
}

func stagePluginLibrary(paths runnerPaths, source string) (string, error) {
	contents, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read plugin library %s: %w", filepath.ToSlash(source), err)
	}
	platformDir, err := preparePluginPlatformDir(paths)
	if err != nil {
		return "", err
	}
	extension, err := pluginExtension(runtime.GOOS)
	if err != nil {
		return "", err
	}
	target := filepath.Join(platformDir, "censorship"+extension)
	if err := os.WriteFile(target, contents, 0o644); err != nil {
		return "", fmt.Errorf("stage plugin library to %s: %w", filepath.ToSlash(target), err)
	}
	return filepath.Join(paths.run, "plugins"), nil
}

func buildPlugin(paths runnerPaths, platformDir string) (string, error) {
	extension, err := pluginExtension(runtime.GOOS)
	if err != nil {
		return "", err
	}
	library := filepath.Join(platformDir, "censorship"+extension)
	if err := runCommand(paths.repositoryRoot, []string{"CGO_ENABLED=1"}, goCommand(), "build", "-trimpath", "-buildmode=c-shared", "-o", library, "."); err != nil {
		return "", err
	}
	header := strings.TrimSuffix(library, extension) + ".h"
	if err := removeContained(paths.integrationRoot, header); err != nil {
		return "", err
	}
	return filepath.Join(paths.run, "plugins"), nil
}

func buildModelMapper(paths runnerPaths, platformDir string) error {
	extension, err := pluginExtension(runtime.GOOS)
	if err != nil {
		return err
	}
	library := filepath.Join(platformDir, "model-mapper"+extension)
	if err := runCommand(paths.mapperCheckout, []string{"CGO_ENABLED=1", "GOOS=" + runtime.GOOS, "GOARCH=" + runtime.GOARCH}, goCommand(), "build", "-mod=readonly", "-trimpath", "-buildmode=c-shared", "-o", library, "."); err != nil {
		return err
	}
	header := strings.TrimSuffix(library, extension) + ".h"
	return removeContained(paths.integrationRoot, header)
}

func copyIntegrationFiles(paths runnerPaths) error {
	sources, err := filepath.Glob(filepath.Join(paths.repositoryRoot, "integration", "*.go"))
	if err != nil {
		return fmt.Errorf("list integration files: %w", err)
	}
	if len(sources) == 0 {
		return fmt.Errorf("no integration Go files found")
	}
	sources = append(sources, filepath.Join(paths.repositoryRoot, ".github", "scripts", "testdata", "abi_benchmark_test.go"))
	destination := filepath.Join(paths.checkout, "integration", "censorshipplugin")
	if err := removeContained(paths.integrationRoot, destination); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("create integration test directory: %w", err)
	}
	for _, source := range sources {
		contents, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read integration file %s: %w", filepath.ToSlash(source), err)
		}
		target := filepath.Join(destination, filepath.Base(source))
		if err := os.WriteFile(target, contents, 0o644); err != nil {
			return fmt.Errorf("copy integration file to %s: %w", filepath.ToSlash(target), err)
		}
	}
	return nil
}

func integrationTestArgs(options runnerOptions) []string {
	args := []string{"test", "-tags=integration", "-count=1", "-v", "./integration/censorshipplugin"}
	if options.abiSmokeLibrary != "" {
		return append(args, "-run", "^TestDynamicABIActiveAfter$")
	}
	if options.benchmark {
		return append(args, "-run", "^$", "-bench", "^BenchmarkDynamicABIRequestInterceptors$", "-benchmem")
	}
	return args
}

func runIntegrationTests(paths runnerPaths, pluginDir string, options runnerOptions) error {
	env := []string{
		"CPA_INTEGRATION_BIN=" + paths.bin,
		"CENSORSHIP_PLUGIN_DIR=" + pluginDir,
	}
	return runCommand(paths.checkout, env, goCommand(), integrationTestArgs(options)...)
}

func goCommand() string {
	if command := os.Getenv("GO"); command != "" {
		return command
	}
	return "go"
}

func runCommand(dir string, extraEnv []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(extraEnv) != 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", strings.Join(append([]string{name}, args...), " "), err)
	}
	return nil
}
