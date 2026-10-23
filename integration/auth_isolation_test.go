//go:build integration

package censorshipintegration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestCPAAuthDirIsolation(t *testing.T) {
	proxy := prepareAmbientAuth(t, "claude-opus")
	var authDirs []string
	for _, enabled := range []bool{true, false} {
		name := "plugins_disabled"
		if enabled {
			name = "plugins_enabled"
		}
		t.Run(name, func(t *testing.T) {
			upstream := newMockUpstream(t)
			localModel := modelName
			var cpa *cpaInstance
			if enabled {
				localModel = "kimi-k3"
				cpa = startCPAWithOptions(t, cpaStartOptions{
					upstreamURL: upstream.URL, pluginsEnabled: true,
					censorshipYAML: "words: {block: [blocked]}\n",
					providerModels: []string{"kimi-k3", "other-model"},
				})
			} else {
				cpa = startCPA(t, upstream.URL, false, "")
			}
			authDir, valid := assertInstanceAuthDir(t, cpa)
			authDirs = append(authDirs, authDir)
			assertAmbientRouteExcluded(t, cpa, upstream, proxy, valid, "/v1/chat/completions", "claude-opus", localModel, localModel)
			if enabled && !slices.Contains(authIsolationModels(t, cpa), "other-model") {
				t.Error("declared other-model missing from model table")
			}
		})
	}
	if len(authDirs) == 2 && authDirs[0] == authDirs[1] {
		t.Errorf("instances share auth-dir %q", authDirs[0])
	}
}

func TestFunctionalDirectAuthDirIsolation(t *testing.T) {
	proxy := prepareAmbientAuth(t, "ambient-direct-only")
	t.Run("claude", func(t *testing.T) {
		upstream := newFunctionalDirectUpstream(t, "claude")
		cpa := startFunctionalDirectCPA(t, upstream.URL, "claude", "words: {block: [blocked]}\n")
		_, valid := assertInstanceAuthDir(t, cpa)
		assertAmbientRouteExcluded(t, cpa, upstream, proxy, valid, "/v1/messages", "ambient-direct-only", modelName, "claude-opus-5-5")
	})
}

func prepareAmbientAuth(t *testing.T, alias string) *upstreamCapture {
	t.Helper()
	runs := filepath.Join(t.TempDir(), "auth isolation runs")
	if err := os.Mkdir(runs, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"GOTMPDIR", "TEMP", "TMP", "TMPDIR"} {
		t.Setenv(key, runs)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	capture := new(upstreamCapture)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture.arrive()
		if r.Method != http.MethodConnect {
			t.Errorf("ambient proxy method = %q, want CONNECT", r.Method)
		}
		http.Error(w, "synthetic auth CONNECT refused", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	authDir := filepath.Join(home, ".cli-proxy-api")
	if err := os.Mkdir(authDir, 0o700); err != nil {
		t.Fatal(err)
	}
	account, err := json.Marshal(map[string]any{
		"type": "claude", "access_token": "synthetic-integration-only", "expired": "2100-01-01T00:00:00Z",
		"proxy_url":     proxy.URL,
		"model_aliases": []map[string]string{{"name": "claude-opus-5-5", "alias": alias}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "ambient.json"), account, 0o600); err != nil {
		t.Fatal(err)
	}
	return capture
}

func assertInstanceAuthDir(t *testing.T, cpa *cpaInstance) (string, bool) {
	t.Helper()
	contents, err := os.ReadFile(cpa.config)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(contents, &config); err != nil {
		t.Fatal(err)
	}
	dir, ok := config["auth-dir"].(string)
	want := filepath.Join(filepath.Dir(cpa.config), "auths")
	valid := ok && filepath.IsAbs(dir) && dir == want
	if !valid {
		t.Errorf("generated auth-dir = %#v, want absolute instance path %q", config["auth-dir"], want)
	}
	if !strings.Contains(want, "auth isolation runs") {
		t.Errorf("run directory did not exercise spaces: %q", want)
	}
	return dir, valid
}

func authIsolationModels(t *testing.T, cpa *cpaInstance) []string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, cpa.baseURL+"/v1/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+downstreamKey)
	response, err := integrationHTTPClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var models struct {
		Data []struct{ ID string } `json:"data"`
	}
	if err := json.Unmarshal(contents, &models); err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("models status=%d body=%s error=%v", response.StatusCode, contents, err)
	}
	var ids []string
	for _, model := range models.Data {
		ids = append(ids, model.ID)
	}
	return ids
}

func assertAmbientRouteExcluded(t *testing.T, cpa *cpaInstance, upstream *mockUpstream, proxy *upstreamCapture, validAuthDir bool, endpoint, alias, localModel, wireModel string) {
	t.Helper()
	started := time.Now()
	deadline := started.Add(10 * time.Second)
	var models []string
	var log string
	for {
		models = authIsolationModels(t, cpa)
		log = readCPALog(cpa.logPath)
		// HTTP readiness 早于账号加载；缺少隔离配置时必须等到真实 alias 注册，保留变异路由证据。
		if strings.Contains(log, "full client load complete") && strings.Contains(log, "file watcher started") && slices.Contains(models, localModel) && (validAuthDir || slices.Contains(models, alias)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("auth loading not observed: valid_auth_dir=%t models=%v\n%s", validAuthDir, models, log)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Logf("auth scan and model registration observed after %s; models=%v", time.Since(started), models)
	if validAuthDir && !strings.Contains(log, "(0 auth files +") {
		t.Errorf("instance scanned unexpected auth files\n%s", log)
	}
	if slices.Contains(models, alias) {
		t.Errorf("ambient alias %q registered in isolated model table", alias)
	}
	body := func(model string) []byte {
		data, err := json.Marshal(map[string]any{
			"model": model, "max_tokens": 16,
			"messages": []map[string]string{{"role": "user", "content": "clean input"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	arrivals, requests, connects := upstream.arrivalCount(), upstream.requestCount(), proxy.arrivalCount()
	status, _, response := postJSON(t, cpa.baseURL+endpoint, body(localModel))
	var captured struct{ Model string }
	if err := json.Unmarshal(upstream.lastRequest(), &captured); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || captured.Model != wireModel || upstream.arrivalCount()-arrivals != 1 || upstream.requestCount()-requests != 1 || proxy.arrivalCount() != connects {
		t.Fatalf("canary status=%d wire_model=%q fixture_arrivals=%d requests=%d proxy_arrivals=%d body=%s", status, captured.Model, upstream.arrivalCount()-arrivals, upstream.requestCount()-requests, proxy.arrivalCount()-connects, response)
	}
	endpoints := []string{endpoint}
	if endpoint != "/v1/chat/completions" {
		endpoints = append(endpoints, "/v1/chat/completions")
	}
	for _, target := range endpoints {
		arrivals, requests, connects = upstream.arrivalCount(), upstream.requestCount(), proxy.arrivalCount()
		status, _, response = postJSON(t, cpa.baseURL+target, body(alias))
		var result struct {
			Error struct{ Code, Type, Message string } `json:"error"`
		}
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatal(err)
		}
		modelNotFound := result.Error.Code == "model_not_found"
		if target == "/v1/messages" {
			modelNotFound = result.Error.Type == "invalid_request_error" && result.Error.Message == "unknown provider for model "+alias
		}
		t.Logf("canary=200; endpoint=%s alias=%s status=%d fixture_arrivals=%d requests=%d proxy_arrivals=%d body=%s", target, alias, status, upstream.arrivalCount()-arrivals, upstream.requestCount()-requests, proxy.arrivalCount()-connects, response)
		if status != http.StatusBadRequest || !modelNotFound || upstream.arrivalCount() != arrivals || upstream.requestCount() != requests || proxy.arrivalCount() != connects {
			t.Errorf("ambient route changed: endpoint=%s status=%d fixture_arrivals=%d requests=%d proxy_arrivals=%d body=%s", target, status, upstream.arrivalCount()-arrivals, upstream.requestCount()-requests, proxy.arrivalCount()-connects, response)
		}
	}
}
