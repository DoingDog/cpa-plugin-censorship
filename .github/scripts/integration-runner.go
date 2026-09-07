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
	cpaSHA    = "c76dfd4e0edabab9000628b1560ab8ab379eadb8"
	cpaRemote = "https://github.com/router-for-me/CLIProxyAPI"
)

type runnerPaths struct {
	repositoryRoot  string
	integrationRoot string
	checkout        string
	bin             string
	run             string
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
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	paths, err := resolveRunnerPaths(root)
	if err != nil {
		return err
	}
	if err := prepareCheckout(paths); err != nil {
		return err
	}
	if err := buildCPA(paths); err != nil {
		return err
	}
	pluginDir, err := buildPlugin(paths)
	if err != nil {
		return err
	}
	if err := copyIntegrationFiles(paths); err != nil {
		return err
	}
	return runIntegrationTests(paths, pluginDir, os.Getenv("BENCH") == "1")
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
		bin:             bin,
		run:             filepath.Join(integrationRoot, "run"),
	}
	for _, path := range []string{paths.checkout, paths.bin, paths.run} {
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
	return nil
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

func prepareCheckout(paths runnerPaths) error {
	if err := verifyCheckout(paths.checkout, cpaSHA); err == nil {
		return nil
	}
	if err := removeContained(paths.integrationRoot, paths.checkout); err != nil {
		return err
	}
	if err := os.MkdirAll(paths.checkout, 0o755); err != nil {
		return fmt.Errorf("create checkout directory: %w", err)
	}
	commands := [][]string{
		{"git", "init"},
		{"git", "remote", "add", "origin", cpaRemote},
		{"git", "fetch", "--depth=1", "origin", cpaSHA},
		{"git", "checkout", "--detach", "FETCH_HEAD"},
	}
	for _, command := range commands {
		if err := runCommand(paths.checkout, nil, command[0], command[1:]...); err != nil {
			return err
		}
	}
	return verifyCheckout(paths.checkout, cpaSHA)
}

func buildCPA(paths runnerPaths) error {
	if err := removeContained(paths.integrationRoot, paths.bin); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(paths.bin), 0o755); err != nil {
		return fmt.Errorf("create CPA binary directory: %w", err)
	}
	return runCommand(paths.checkout, nil, "go", "build", "-trimpath", "-o", paths.bin, "./cmd/server")
}

func buildPlugin(paths runnerPaths) (string, error) {
	extension, err := pluginExtension(runtime.GOOS)
	if err != nil {
		return "", err
	}
	pluginDir := filepath.Join(paths.run, "plugins")
	platformDir := filepath.Join(pluginDir, runtime.GOOS, runtime.GOARCH)
	if err := requireContained(paths.integrationRoot, platformDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(platformDir, 0o755); err != nil {
		return "", fmt.Errorf("create plugin directory: %w", err)
	}
	library := filepath.Join(platformDir, "censorship"+extension)
	if err := runCommand(paths.repositoryRoot, []string{"CGO_ENABLED=1"}, "go", "build", "-trimpath", "-buildmode=c-shared", "-o", library, "."); err != nil {
		return "", err
	}
	header := strings.TrimSuffix(library, extension) + ".h"
	if err := removeContained(paths.integrationRoot, header); err != nil {
		return "", err
	}
	return pluginDir, nil
}

func copyIntegrationFiles(paths runnerPaths) error {
	sources, err := filepath.Glob(filepath.Join(paths.repositoryRoot, "integration", "*.go"))
	if err != nil {
		return fmt.Errorf("list integration files: %w", err)
	}
	if len(sources) == 0 {
		return fmt.Errorf("no integration Go files found")
	}
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

func runIntegrationTests(paths runnerPaths, pluginDir string, bench bool) error {
	args := []string{"test", "-tags=integration", "-count=1", "-v", "./integration/censorshipplugin"}
	if bench {
		args = append(args, "-run", "^$", "-bench", ".", "-benchmem")
	}
	env := []string{
		"CPA_INTEGRATION_BIN=" + paths.bin,
		"CENSORSHIP_PLUGIN_DIR=" + pluginDir,
	}
	return runCommand(paths.checkout, env, "go", args...)
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
