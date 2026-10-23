//go:build integration

package censorshipintegration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pluginhost"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

func TestLifecycleNativeRecovery(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) { runLifecycleProcess(t, "recovery", phase) })
	}
}

func TestLifecycleNativeInvalidRemainsInactive(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) { runLifecycleProcess(t, "invalid-recovery", phase) })
	}
}

func TestLifecycleNativeEstablishedPhases(t *testing.T) {
	for _, scenario := range []string{"initial", "zero-rules"} {
		for _, phase := range []string{"before", "after"} {
			t.Run(scenario+"/"+phase, func(t *testing.T) { runLifecycleProcess(t, scenario, phase) })
		}
	}
}

func runLifecycleProcess(t *testing.T, scenario, phase string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestLifecycleNativeProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "CENSORSHIP_LIFECYCLE_CASE="+scenario, "CENSORSHIP_LIFECYCLE_PHASE="+phase)
	output, err := cmd.CombinedOutput()
	t.Logf("fresh process %s/%s\n%s", scenario, phase, output)
	if err != nil {
		t.Fatalf("native child: %v, deadline=%v", err, ctx.Err())
	}
}

func lifecycleNativeConfig(t *testing.T, phase, words string) *config.Config {
	t.Helper()
	pluginDir := os.Getenv("CENSORSHIP_PLUGIN_DIR")
	if pluginDir == "" || !filepath.IsAbs(pluginDir) {
		t.Fatalf("absolute CENSORSHIP_PLUGIN_DIR required: %q", pluginDir)
	}
	raw := fmt.Sprintf("plugins:\n  enabled: true\n  dir: %q\n  configs:\n    censorship:\n      enabled: true\n", pluginDir)
	if words == "" {
		raw += "      words: {}\n"
	} else {
		raw += fmt.Sprintf("      words: {block: [%s]}\n", words)
	}
	if phase == "after" {
		raw += "      filter_mode: include\n      filter: {models: [target-model]}\n"
	}
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	return &cfg
}

func requireLifecycleRegistered(t *testing.T, host *pluginhost.Host, want bool) {
	t.Helper()
	found := false
	for _, info := range host.RegisteredPlugins() {
		if info.ID == "censorship" {
			found = true
		}
	}
	if got := host.PluginRegistered("censorship"); got != want || found != want {
		t.Fatalf("PluginRegistered=%t, RegisteredPlugins censorship=%t, want=%t", got, found, want)
	}
}

func requireLifecycleNativeBlock(t *testing.T, host *pluginhost.Host, phase, term string, active bool) {
	t.Helper()
	request := pluginapi.RequestInterceptRequest{
		RequestID: "lifecycle-native", SourceFormat: "openai", Model: "target-model", RequestedModel: "requested-decoy",
		Metadata: map[string]any{executor.SelectedAuthMetadataKey: "auth-1", "sentinel": "BLOCKME metadata"},
		Headers:  http.Header{"X-Preserved": {"BLOCKME header"}},
		Body:     []byte(`{"model":"body-decoy","messages":[{"role":"user","content":"` + term + `"}],"metadata":{"sentinel":"BLOCKME"},"temperature":1.2300e+00}`),
	}
	original, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"before", "after"} {
		var response pluginapi.RequestInterceptResponse
		if method == "before" {
			response = host.InterceptRequestBeforeAuth(context.Background(), request)
		} else {
			response = host.InterceptRequestAfterAuth(context.Background(), request)
		}
		if active && method == phase {
			decoded, err := decodeCensorshipError(response.ResponseBody)
			if err != nil || !response.Terminate || response.StatusCode != 400 || response.ResponseHeaders.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != term || decoded.Error.Role != "user" || !bytes.Equal(response.Body, request.Body) || !reflect.DeepEqual(response.Headers, request.Headers) || len(response.ClearHeaders) != 0 {
				t.Fatalf("%s block=%#v, error=%v", method, response, err)
			}
		} else if !reflect.DeepEqual(response, pluginapi.RequestInterceptResponse{Body: request.Body, Headers: request.Headers}) {
			t.Fatalf("%s no-op=%#v", method, response)
		}
	}
	final, err := json.Marshal(request)
	if err != nil || !bytes.Equal(original, final) {
		t.Fatalf("native body/headers/metadata mutated, error=%v", err)
	}
	if active && phase == "after" {
		for _, missingMarker := range []bool{false, true} {
			bypass := request
			if missingMarker {
				bypass.Metadata = nil
			} else {
				bypass.Model = "attempt-decoy"
				bypass.RequestedModel = "target-model"
				bypass.Body = bytes.Replace(request.Body, []byte(`"body-decoy"`), []byte(`"target-model"`), 1)
			}
			before, err := json.Marshal(bypass)
			if err != nil {
				t.Fatal(err)
			}
			response := host.InterceptRequestAfterAuth(context.Background(), bypass)
			if !reflect.DeepEqual(response, pluginapi.RequestInterceptResponse{Body: bypass.Body, Headers: bypass.Headers}) {
				t.Fatalf("native after gate no-op=%#v", response)
			}
			after, err := json.Marshal(bypass)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("native after gate mutated input, error=%v", err)
			}
		}
	}
}

func TestLifecycleNativeProcess(t *testing.T) {
	scenario := os.Getenv("CENSORSHIP_LIFECYCLE_CASE")
	if scenario == "" {
		return
	}
	phase := os.Getenv("CENSORSHIP_LIFECYCLE_PHASE")
	extension := ".so"
	if runtime.GOOS == "windows" {
		extension = ".dll"
	} else if runtime.GOOS == "darwin" {
		extension = ".dylib"
	}
	library := filepath.Join(os.Getenv("CENSORSHIP_PLUGIN_DIR"), runtime.GOOS, runtime.GOARCH, "censorship"+extension)
	contents, err := os.ReadFile(library)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("loaded library=%s sha256=%x", library, sha256.Sum256(contents))
	host := pluginhost.New()
	t.Cleanup(host.ShutdownAll)
	if scenario == "initial" || scenario == "zero-rules" {
		words := "BLOCKME"
		if scenario == "zero-rules" {
			words = ""
		}
		host.ApplyConfig(context.Background(), lifecycleNativeConfig(t, phase, words))
		requireLifecycleRegistered(t, host, true)
		requireLifecycleNativeBlock(t, host, phase, "BLOCKME", scenario == "initial")
		for _, invalid := range []string{"''", "123"} {
			host.ApplyConfig(context.Background(), lifecycleNativeConfig(t, phase, invalid))
			requireLifecycleRegistered(t, host, true)
			requireLifecycleNativeBlock(t, host, phase, "BLOCKME", scenario == "initial")
		}
		opposite := "before"
		if phase == "before" {
			opposite = "after"
		}
		host.ApplyConfig(context.Background(), lifecycleNativeConfig(t, opposite, "new-block"))
		requireLifecycleRegistered(t, host, true)
		requireLifecycleNativeBlock(t, host, phase, "BLOCKME", scenario == "initial")
		requireLifecycleNativeBlock(t, host, phase, "new-block", false)
		host.ApplyConfig(context.Background(), lifecycleNativeConfig(t, phase, "new-block"))
		requireLifecycleRegistered(t, host, true)
		requireLifecycleNativeBlock(t, host, phase, "new-block", true)
		requireLifecycleNativeBlock(t, host, phase, "BLOCKME", false)
		return
	}
	if scenario != "recovery" && scenario != "invalid-recovery" {
		t.Fatalf("unknown lifecycle case %q", scenario)
	}
	host.ApplyConfig(context.Background(), lifecycleNativeConfig(t, phase, "''"))
	requireLifecycleRegistered(t, host, false)
	requireLifecycleNativeBlock(t, host, phase, "BLOCKME", false)
	if scenario == "invalid-recovery" {
		for _, invalid := range []string{"''", "123"} {
			host.ApplyConfig(context.Background(), lifecycleNativeConfig(t, phase, invalid))
			requireLifecycleRegistered(t, host, false)
			requireLifecycleNativeBlock(t, host, phase, "BLOCKME", false)
		}
	}
	for range 2 {
		host.ApplyConfig(context.Background(), lifecycleNativeConfig(t, phase, "BLOCKME"))
		requireLifecycleRegistered(t, host, true)
		requireLifecycleNativeBlock(t, host, phase, "BLOCKME", true)
	}
}

func TestLifecycleWatcherRecovery(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			upstream := newMockUpstream(t)
			filter := ""
			if phase == "after" {
				filter = "filter_mode: include\nfilter: {models: [censorship-integration-model]}\n"
			}
			cpa := startCPA(t, upstream.URL, true, filter+"words: {block: ['']}\n")
			if log := readCPALog(cpa.logPath); !strings.Contains(log, "register") || !strings.Contains(log, "empty") {
				t.Fatalf("first register rejection not observed\n%s", log)
			}
			arrivals, requests := upstream.arrivalCount(), upstream.requestCount()
			status, _, body := postChat(t, cpa, "clean canary")
			if status != 200 || upstream.arrivalCount() != arrivals+1 || upstream.requestCount() != requests+1 {
				t.Fatalf("clean canary status=%d body=%s", status, body)
			}
			t.Logf("clean canary status=%d arrival delta=1 request delta=1", status)
			legal := filter + "words: {block: [BLOCKME, watcher-ready-only]}\n"
			timer := time.NewTimer(20 * time.Second)
			ticker := time.NewTicker(time.Second)
			defer timer.Stop()
			defer ticker.Stop()
			for {
				writePluginConfig(t, cpa.config, legal)
				status, _, body = postChat(t, cpa, "watcher-ready-only")
				decoded, err := decodeCensorshipError(body)
				if status == 400 && err == nil && decoded.Error.Code == "censorship_blocked" && decoded.Error.Term == "watcher-ready-only" {
					break
				}
				select {
				case <-ticker.C:
				case <-timer.C:
					t.Fatalf("watcher recovery not observed, status=%d body=%s\n%s", status, body, readCPALog(cpa.logPath))
				}
			}
			arrivals, requests = upstream.arrivalCount(), upstream.requestCount()
			status, headers, body := postChat(t, cpa, "BLOCKME")
			decoded, err := decodeCensorshipError(body)
			if err != nil || status != 400 || !strings.Contains(headers.Get("Content-Type"), "application/json") || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != "BLOCKME" || decoded.Error.Role != "user" || upstream.arrivalCount() != arrivals || upstream.requestCount() != requests {
				t.Fatalf("recovered HTTP status=%d body=%s, arrival delta=%d request delta=%d error=%v", status, body, upstream.arrivalCount()-arrivals, upstream.requestCount()-requests, err)
			}
			t.Logf("watcher observed recovery; BLOCKME status=%d code=%s arrival delta=0 request delta=0", status, decoded.Error.Code)
		})
	}
}
