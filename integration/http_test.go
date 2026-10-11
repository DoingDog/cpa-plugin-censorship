//go:build integration

package censorshipintegration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"gopkg.in/yaml.v3"
)

type integrationCensorshipErrorResponse struct {
	Error struct {
		Code string `json:"code"`
		Term string `json:"term"`
		Role string `json:"role"`
	} `json:"error"`
}

func decodeCensorshipError(body []byte) (integrationCensorshipErrorResponse, error) {
	var raw struct {
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return integrationCensorshipErrorResponse{}, err
	}
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return integrationCensorshipErrorResponse{}, fmt.Errorf("decode censorship error: top-level null")
	}
	if len(raw.Error) > 0 {
		if bytes.Equal(bytes.TrimSpace(raw.Error), []byte("null")) {
			return integrationCensorshipErrorResponse{}, fmt.Errorf("decode censorship error: error is null")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw.Error, &fields); err != nil {
			return integrationCensorshipErrorResponse{}, err
		}
		for _, name := range []string{"code", "term", "role"} {
			if value, ok := fields[name]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return integrationCensorshipErrorResponse{}, fmt.Errorf("decode censorship error: error.%s is null", name)
			}
		}
	}

	var response integrationCensorshipErrorResponse
	err := json.Unmarshal(body, &response)
	return response, err
}

func TestDecodeCensorshipError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    []byte
		want    integrationCensorshipErrorResponse
		wantErr bool
	}{
		{
			name: "complete JSON",
			body: []byte(`{"error":{"code":"censorship_blocked","term":"Alpha","role":"user"}}`),
			want: integrationCensorshipErrorResponse{Error: struct {
				Code string `json:"code"`
				Term string `json:"term"`
				Role string `json:"role"`
			}{Code: "censorship_blocked", Term: "Alpha", Role: "user"}},
		},
		{
			name:    "missing closing braces",
			body:    []byte(`{"error":{"code":"censorship_blocked","term":"Alpha","role":"user"`),
			wantErr: true,
		},
		{
			name:    "trailing non-whitespace",
			body:    []byte(`{"error":{"code":"censorship_blocked","term":"Alpha","role":"user"}}x`),
			wantErr: true,
		},
		{
			name:    "numeric error term",
			body:    []byte(`{"error":{"code":"censorship_blocked","term":1,"role":"user"}}`),
			wantErr: true,
		},
		{
			name:    "top-level null",
			body:    []byte(`null`),
			wantErr: true,
		},
		{
			name:    "null error",
			body:    []byte(`{"error":null}`),
			wantErr: true,
		},
		{
			name:    "null error code",
			body:    []byte(`{"error":{"code":null}}`),
			wantErr: true,
		},
		{
			name:    "null error term",
			body:    []byte(`{"error":{"term":null}}`),
			wantErr: true,
		},
		{
			name:    "null error role",
			body:    []byte(`{"error":{"role":null}}`),
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeCensorshipError(tc.body)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("decodeCensorshipError(%s) unexpectedly succeeded", tc.body)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("decodeCensorshipError(%s) = %#v, want %#v", tc.body, got, tc.want)
			}
		})
	}
}

func TestHTTPBlockIncludesTermAndRole(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [Alpha]\nignore_case: true\n")
	status, header, body := postJSON(t, cpa.baseURL+"/v1/chat/completions", []byte(`{"model":"censorship-integration-model","messages":[{"role":"user","content":"aLPHA"}]}`))
	errorResponse, err := decodeCensorshipError(body)
	if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || errorResponse.Error.Term != "Alpha" || errorResponse.Error.Role != "user" {
		t.Fatalf("status=%d header=%v body=%s", status, header, body)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked request reached upstream")
	}
}

func TestHTTPCleanMappedCanaryUsesConfiguredProviderModels(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPAWithOptions(t, cpaStartOptions{
		upstreamURL:    upstream.URL,
		pluginsEnabled: true,
		modelMapperYAML: `priority: 999
openai_completions_rules: 'claude-opus=>kimi-k3;claude-control=>other-model'
`,
		censorshipYAML: `priority: 800
words:
  block: [blocked]
filter_mode: exclude
filter:
  models: [kimi-k3]
`,
		providerModels: []string{"kimi-k3", "other-model"},
	})
	for _, tc := range []struct {
		name          string
		source        string
		content       string
		wantStatus    int
		wantModel     string
		wantArrivals  int
		wantOuterCode string
		checkContent  bool
	}{
		{name: "direct excluded model bypasses censorship", source: "kimi-k3", content: "blocked", wantStatus: 200, wantModel: "kimi-k3", wantArrivals: 1, checkContent: true},
		{name: "mapped excluded model clean route", source: "claude-opus", content: "clean input", wantStatus: 200, wantModel: "kimi-k3", wantArrivals: 1, checkContent: true},
		{name: "mapped excluded model bypasses censorship", source: "claude-opus", content: "blocked", wantStatus: 200, wantModel: "kimi-k3", wantArrivals: 1, checkContent: true},
		{name: "mapped nonexcluded clean control", source: "claude-control", content: "clean input", wantStatus: 200, wantModel: "other-model", wantArrivals: 1},
		{name: "mapped nonexcluded block control", source: "claude-control", content: "blocked", wantStatus: 500, wantArrivals: 0, wantOuterCode: "internal_server_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := upstream.arrivalCount()
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":%q}]}`, tc.source, tc.content))
			status, _, response := postJSON(t, cpa.baseURL+"/v1/chat/completions", body)
			if status != tc.wantStatus {
				t.Fatalf("source model %q status=%d, want %d; body=%s", tc.source, status, tc.wantStatus, response)
			}
			if got, want := upstream.arrivalCount(), before+tc.wantArrivals; got != want {
				t.Fatalf("source model %q upstream arrivals=%d, want %d", tc.source, got, want)
			}
			if tc.wantOuterCode != "" && gjson.GetBytes(response, "error.code").String() != tc.wantOuterCode {
				t.Fatalf("source model %q outer error code=%q, want %q; body=%s", tc.source, gjson.GetBytes(response, "error.code").String(), tc.wantOuterCode, response)
			}
			if tc.wantModel != "" {
				if got := gjson.GetBytes(upstream.lastRequest(), "model").String(); got != tc.wantModel {
					t.Fatalf("source model %q upstream model=%q, want %q", tc.source, got, tc.wantModel)
				}
			}
			if tc.checkContent {
				if got := gjson.GetBytes(upstream.lastRequest(), "messages.0.content").String(); got != tc.content {
					t.Fatalf("source model %q upstream content=%q, want %q", tc.source, got, tc.content)
				}
			}
		})
	}
}

func TestHTTPMappedModelNotFoundWithoutMapper(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPAWithOptions(t, cpaStartOptions{
		upstreamURL:    upstream.URL,
		pluginsEnabled: true,
		censorshipYAML: `priority: 800
words:
  block: [blocked]
filter_mode: exclude
filter:
  models: [kimi-k3]
`,
		providerModels: []string{"kimi-k3", "other-model"},
	})
	status, _, response := postJSON(t, cpa.baseURL+"/v1/chat/completions", []byte(`{"model":"claude-opus","messages":[{"role":"user","content":"clean input"}]}`))
	if status >= 200 && status < 300 {
		t.Fatalf("mapper-disabled claude-opus status=%d, want model-not-found failure; body=%s", status, response)
	}
	if !bytes.Contains(response, []byte("claude-opus")) {
		t.Fatalf("mapper-disabled response does not identify claude-opus: %s", response)
	}
	if got := gjson.GetBytes(response, "error.code").String(); got != "model_not_found" {
		t.Fatalf("mapper-disabled error code=%q, want model_not_found; body=%s", got, response)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatalf("mapper-disabled claude-opus reached upstream %d times", upstream.arrivalCount())
	}
}

func TestHTTPRequestFilterMatchesAllSupportedFormats(t *testing.T) {
	const config = `words:
  block: [SECRET]
filter_mode: include
filter_logic: and
filter:
  api-keys: ["censorship-integration-?ey"]
  models: ["censorship-integration-*"]
`
	for _, tc := range []struct {
		name string
		path string
		body []byte
	}{
		{name: "OpenAI Chat", path: "/v1/chat/completions", body: []byte(`{"model":"censorship-integration-model","messages":[{"role":"user","content":"SECRET"}]}`)},
		{name: "Responses", path: "/v1/responses", body: []byte(`{"model":"censorship-integration-model","input":"SECRET"}`)},
		{name: "Claude", path: "/v1/messages", body: []byte(`{"model":"censorship-integration-model","max_tokens":16,"messages":[{"role":"user","content":"SECRET"}]}`)},
		{name: "Gemini", path: "/v1beta/models/censorship-integration-model:generateContent", body: []byte(`{"contents":[{"role":"user","parts":[{"text":"SECRET"}]}]}`)},
		{name: "Interactions", path: "/v1beta/interactions", body: []byte(`{"model":"censorship-integration-model","input":"SECRET"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := newMockUpstream(t)
			cpa := startCPA(t, upstream.URL, true, config)
			status, _, body := postJSON(t, cpa.baseURL+tc.path, tc.body)
			errorResponse, err := decodeCensorshipError(body)
			if err != nil || status != 400 || errorResponse.Error.Code != "censorship_blocked" {
				t.Fatalf("status=%d body=%s error=%v", status, body, err)
			}
			if upstream.arrivalCount() != 0 {
				t.Fatal("blocked request reached upstream")
			}
		})
	}
}

func TestHTTPRejectsDuplicateJSONMembers(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: strip\nwords: [SECRET]\n")
	body := []byte(`{"model":"censorship-integration-model","messages":[],"messages":[{"role":"user","content":"SECRET"}]}`)
	status, header, response := postJSON(t, cpa.baseURL+"/v1/chat/completions", body)
	errorResponse, err := decodeCensorshipError(response)
	if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || errorResponse.Error.Code != "censorship_invalid_request" {
		t.Fatalf("status=%d header=%v body=%s", status, header, response)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("duplicate-member request reached upstream")
	}
}

func TestLegacyCompletionsPromptUsesConvertedUserRole(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nignore_case: true\nwords: [Alpha]\n")
	status, _, body := postJSON(t, cpa.baseURL+"/v1/completions", []byte(`{"model":"censorship-integration-model","prompt":"aLPHA legacy prompt"}`))
	errorResponse, err := decodeCensorshipError(body)
	if err != nil || status != 400 || errorResponse.Error.Term != "Alpha" || errorResponse.Error.Role != "user" {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked legacy prompt reached upstream")
	}
}

func TestWatcherReloadLinearizesAtObservedSnapshotB(t *testing.T) {
	const probe = "watcher-probe"
	const readyProbe = "watcher-ready-only"
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, `words:
  block: [snapshot-a-only]
filter_mode: include
filter:
  models: [never-match]
`)

	waitFor := func(name string, interval time.Duration, condition func() bool) {
		timer := time.NewTimer(20 * time.Second)
		ticker := time.NewTicker(interval)
		defer timer.Stop()
		defer ticker.Stop()
		for {
			if condition() {
				return
			}
			select {
			case <-ticker.C:
			case <-timer.C:
				t.Fatalf("%s\n%s", name, readCPALog(cpa.logPath))
			}
		}
	}

	requestsBeforeA := upstream.requestCount()
	status, _, body := postChat(t, cpa, probe)
	if status != 200 {
		t.Fatalf("snapshot A did not bypass probe: status=%d body=%s", status, body)
	}
	if got, want := upstream.requestCount(), requestsBeforeA+1; got != want {
		t.Fatalf("snapshot A bypass request count = %d, want %d", got, want)
	}

	waitFor("config watcher not observed", time.Second, func() bool {
		writePluginConfig(t, cpa.config, `words:
  block: [snapshot-a-only, watcher-ready-only]
filter_mode: include
filter:
  models: [censorship-integration-model]
`)
		status, _, body = postChat(t, cpa, readyProbe)
		return status == 400 && gjson.GetBytes(body, "error.term").String() == readyProbe
	})

	writePluginConfig(t, cpa.config, `words:
  block: [watcher-probe]
filter_mode: include
filter:
  models: [censorship-integration-model]
`)
	waitFor("snapshot B not observed", 25*time.Millisecond, func() bool {
		status, _, body = postChat(t, cpa, probe)
		return status == 400 && gjson.GetBytes(body, "error.term").String() == probe
	})

	logOffset := len(readCPALog(cpa.logPath))
	writePluginConfig(t, cpa.config, `words:
  block: [invalid-only]
filter_mode: include
filter_logic: xor
filter:
  models: [never-match]
`)
	waitFor("invalid snapshot rejection not observed", 25*time.Millisecond, func() bool {
		log := readCPALog(cpa.logPath)
		return len(log) >= logOffset && strings.Contains(log[logOffset:], "censorship plugin reconfigure rejected")
	})

	status, _, body = postChat(t, cpa, probe)
	if status != 400 || gjson.GetBytes(body, "error.term").String() != probe {
		t.Fatalf("invalid reconfigure replaced snapshot B: status=%d body=%s", status, body)
	}
}

func TestHTTPResponsesBlockStringInput(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [SECRET]\n")
	status, _, body := postJSON(t, cpa.baseURL+"/v1/responses", []byte(`{"model":"censorship-integration-model","input":"SECRET"}`))
	if status != 400 {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked Responses input reached upstream")
	}
}

func TestHTTPResponsesDeveloperInstructionsBlock(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
	status, _, body := postJSON(t, cpa.baseURL+"/v1/responses", []byte(`{"model":"censorship-integration-model","instructions":"SECRET","input":"safe"}`))
	response, err := decodeCensorshipError(body)
	if err != nil || status != 400 || response.Error.Code != "censorship_blocked" || response.Error.Term != "SECRET" || response.Error.Role != "developer" {
		t.Fatalf("status=%d body=%s error=%v", status, body, err)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked Responses instructions reached upstream")
	}
}

func TestHTTPResponsesNestedFunctionDescriptionsBlock(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "top-level tools function description",
			body: `{"model":"censorship-integration-model","input":"safe","tools":[{"type":"function","function":{"name":"lookup","description":"SECRET lookup","parameters":{"type":"object","properties":{"query":{"type":"string","description":"safe"}}}}}]}`,
		},
		{
			name: "developer additional_tools schema description",
			body: `{"model":"censorship-integration-model","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"safe"}]},{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"lookup","function":{"description":"safe","parameters":{"type":"object","properties":{"query":{"type":"string","description":"SECRET query"}}}}}]}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := postJSON(t, cpa.baseURL+"/v1/responses", []byte(tc.body))
			response, err := decodeCensorshipError(body)
			if err != nil || status != 400 || response.Error.Code != "censorship_blocked" || response.Error.Role != "developer" {
				t.Fatalf("status=%d body=%s error=%v", status, body, err)
			}
			if upstream.arrivalCount() != 0 {
				t.Fatal("blocked Responses function reached upstream")
			}
		})
	}
}

func TestHTTPResponsesNestedFunctionDescriptionsStrip(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  strip: [SECRET]\nscope:\n  roles: [developer]\n")
	body := []byte(`{"model":"censorship-integration-model","input":"safe","tools":[{"type":"function","function":{"name":"SECRET_lookup","description":"before SECRET after","parameters":{"type":"object","properties":{"query":{"type":"string","description":"SECRET query","enum":["SECRET"]}}}}}]}`)
	status, _, response := postJSON(t, cpa.baseURL+"/v1/responses", body)
	if status != 200 || upstream.arrivalCount() != 1 {
		t.Fatalf("status=%d body=%s upstream arrivals=%d", status, response, upstream.arrivalCount())
	}
	captured := upstream.lastRequest()
	for path, want := range map[string]string{
		"tools.0.function.description":                             "before  after",
		"tools.0.function.parameters.properties.query.description": " query",
		"tools.0.function.name":                                    "SECRET_lookup",
		"tools.0.function.parameters.properties.query.type":        "string",
		"tools.0.function.parameters.properties.query.enum.0":      "SECRET",
	} {
		if got := gjson.GetBytes(captured, path).String(); got != want {
			t.Errorf("upstream %s = %q, want %q; body = %s", path, got, want, captured)
		}
	}
}

func TestHTTPResponsesStripActivatesNestedDescription(t *testing.T) {
	t.Run("strips effective description", func(t *testing.T) {
		upstream := newMockUpstream(t)
		cpa := startCPA(t, upstream.URL, true, "words:\n  strip: [SECRET]\nscope:\n  roles: [developer]\n")
		body := []byte(`{"model":"censorship-integration-model","input":"safe","tools":[{"type":"function","description":"SECRET","function":{"name":"SECRET_lookup","description":"SECRET nested","parameters":{"type":"object","properties":{"query":{"type":"string","enum":["SECRET machine"]}}}}}]}`)
		status, _, response := postJSON(t, cpa.baseURL+"/v1/responses", body)
		if status != 200 || upstream.arrivalCount() != 1 {
			t.Fatalf("status=%d body=%s upstream arrivals=%d", status, response, upstream.arrivalCount())
		}
		captured := upstream.lastRequest()
		for path, want := range map[string]string{
			"tools.0.function.description":                        " nested",
			"tools.0.function.name":                               "SECRET_lookup",
			"tools.0.function.parameters.properties.query.type":   "string",
			"tools.0.function.parameters.properties.query.enum.0": "SECRET machine",
		} {
			if got := gjson.GetBytes(captured, path).String(); got != want {
				t.Errorf("upstream %s = %q, want %q; body = %s", path, got, want, captured)
			}
		}
	})

	t.Run("blocks activated description", func(t *testing.T) {
		upstream := newMockUpstream(t)
		cpa := startCPA(t, upstream.URL, true, "words:\n  block: [BLOCK]\n  strip: [SECRET]\nscope:\n  roles: [developer]\n")
		body := []byte(`{"model":"censorship-integration-model","input":"safe","tools":[{"type":"function","description":"SECRET","function":{"name":"lookup","description":"BLOCK nested"}}]}`)
		status, _, response := postJSON(t, cpa.baseURL+"/v1/responses", body)
		errorResponse, err := decodeCensorshipError(response)
		if err != nil || status != 400 || errorResponse.Error.Code != "censorship_blocked" || errorResponse.Error.Term != "BLOCK" || errorResponse.Error.Role != "developer" {
			t.Fatalf("status=%d body=%s error=%v", status, response, err)
		}
		if upstream.arrivalCount() != 0 {
			t.Fatal("blocked activated Responses description reached upstream")
		}
	})
}

func TestHTTPResponsesAnnotatedReplayRejectsRewrite(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  strip: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"model":"censorship-integration-model","input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"SECRET citation","annotations":[{"type":"url_citation","start_index":7,"end_index":15,"title":"source","url":"https://example.com"}]}]}]}`)
	status, _, response := postJSON(t, cpa.baseURL+"/v1/responses", body)
	errorResponse, err := decodeCensorshipError(response)
	if err != nil || status != 400 || errorResponse.Error.Code != "censorship_invalid_request" {
		t.Fatalf("status=%d body=%s error=%v", status, response, err)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("invalid Responses annotated replay reached upstream")
	}
}

func TestHTTPRequestFilterStripPreservesNonTargetFields(t *testing.T) {
	const input = "before SECRET after"
	const transformed = "before  after"
	body := []byte(`{"model":"censorship-integration-model","messages":[{"role":"user","content":"before SECRET after"}]}`)
	baselineUpstream := newMockUpstream(t)
	transformedUpstream := newMockUpstream(t)
	baseline := startCPA(t, baselineUpstream.URL, false, "")
	transformedCPA := startCPA(t, transformedUpstream.URL, true, `words:
  strip: [SECRET]
filter_mode: include
filter_logic: and
filter:
  api-keys: ["censorship-integration-?ey"]
  models: ["censorship-integration-*"]
`)
	status, _, response := postJSON(t, baseline.baseURL+"/v1/chat/completions", body)
	if status != 200 {
		t.Fatalf("disabled status=%d body=%s", status, response)
	}
	status, _, response = postJSON(t, transformedCPA.baseURL+"/v1/chat/completions", body)
	if status != 200 {
		t.Fatalf("strip status=%d body=%s", status, response)
	}
	if err := validateCapturedChatRequest(transformedUpstream.lastRequest(), baselineUpstream.lastRequest(), input, transformed); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPRequestFilterBypassMatchesDisabledPlugin(t *testing.T) {
	const input = "before SECRET after"
	body := []byte(`{"model":"censorship-integration-model","messages":[{"role":"user","content":"before SECRET after"}]}`)
	disabledUpstream := newMockUpstream(t)
	enabledUpstream := newMockUpstream(t)
	disabled := startCPA(t, disabledUpstream.URL, false, "")
	enabled := startCPA(t, enabledUpstream.URL, true, `words:
  block: [SECRET]
filter_mode: include
filter:
  models: ["never-*"]
`)
	disabledTrace := captureHTTP11Trace(t, disabled.baseURL+"/v1/chat/completions", downstreamKey, body)
	enabledTrace := captureHTTP11Trace(t, enabled.baseURL+"/v1/chat/completions", downstreamKey, body)
	if err := validateCapturedChatRequest(enabledUpstream.lastRequest(), disabledUpstream.lastRequest(), input, input); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(enabledTrace, disabledTrace) {
		t.Fatalf("enabled trace = %#v, disabled trace = %#v", enabledTrace, disabledTrace)
	}
	if enabledUpstream.arrivalCount() != 1 || disabledUpstream.arrivalCount() != 1 {
		t.Fatalf("enabled arrivals=%d disabled arrivals=%d, want 1 each", enabledUpstream.arrivalCount(), disabledUpstream.arrivalCount())
	}
}

func TestHTTPClaudeScalarUserFullStripReturnsLocalInvalidRequest(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  strip: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"model":"censorship-integration-model","max_tokens":16,"messages":[{"role":"user","content":"SECRET"}]}`)
	status, header, response := postJSON(t, cpa.baseURL+"/v1/messages", body)
	errorResponse, err := decodeCensorshipError(response)
	if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || errorResponse.Error.Code != "censorship_invalid_request" {
		t.Fatalf("status=%d header=%v body=%s", status, header, response)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("invalid Claude rewrite reached upstream")
	}
}

func TestHTTPGeminiSignedHistoryBlockReturnsLocalError(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"contents":[{"role":"model","parts":[{"text":"SECRET signed history","thoughtSignature":"c2ln"}]}]}`)
	status, _, response := postJSON(t, cpa.baseURL+"/v1beta/models/censorship-integration-model:generateContent", body)
	errorResponse, err := decodeCensorshipError(response)
	if err != nil || status != 400 || errorResponse.Error.Code != "censorship_blocked" || errorResponse.Error.Term != "SECRET" || errorResponse.Error.Role != "assistant" {
		t.Fatalf("status=%d body=%s", status, response)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked signed Gemini history reached upstream")
	}
}

func TestHTTPGeminiFunctionResponseUsesUserScope(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  block: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"safe"}]},{"parts":[{"functionResponse":{"name":"lookup","response":{"result":"safe"}}},{"text":"SECRET answer"}]}]}`)
	status, _, responseBody := postJSON(t, cpa.baseURL+"/v1beta/models/censorship-integration-model:generateContent", body)
	response, err := decodeCensorshipError(responseBody)
	if err != nil || status != 400 || response.Error.Code != "censorship_blocked" || response.Error.Role != "user" {
		t.Fatalf("status=%d body=%s error=%v", status, responseBody, err)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked Gemini function response reached upstream")
	}
}

func TestHTTPGeminiSnakeFunctionDeclarationBlocks(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"safe"}]}],"tools":[{"function_declarations":[{"name":"lookup","description":"SECRET lookup"}]}]}`)
	status, _, response := postJSON(t, cpa.baseURL+"/v1beta/models/censorship-integration-model:generateContent", body)
	errorResponse, err := decodeCensorshipError(response)
	if err != nil || status != 400 || errorResponse.Error.Code != "censorship_blocked" || errorResponse.Error.Role != "developer" {
		t.Fatalf("status=%d body=%s error=%v", status, response, err)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked Gemini function declaration reached upstream")
	}
}

func TestHTTPGeminiResponseSchemaDescriptionsBlock(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "words:\n  block: [SECRET]\nscope:\n  roles: [developer]\n")
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "generationConfig.responseSchema",
			body: `{"contents":[{"role":"user","parts":[{"text":"safe"}]}],"generationConfig":{"responseMimeType":"application/json","responseSchema":{"type":"OBJECT","properties":{"answer":{"type":"STRING","description":"SECRET answer"}}}}}`,
		},
		{
			name: "generationConfig.responseJsonSchema",
			body: `{"contents":[{"role":"user","parts":[{"text":"safe"}]}],"generationConfig":{"responseMimeType":"application/json","responseJsonSchema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET answer"}}}}}`,
		},
		{
			name: "generationConfig.responseFormat.text.schema",
			body: `{"contents":[{"role":"user","parts":[{"text":"safe"}]}],"generationConfig":{"responseFormat":{"text":{"mimeType":"application/json","schema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET answer"}}}}}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := postJSON(t, cpa.baseURL+"/v1beta/models/censorship-integration-model:generateContent", []byte(tc.body))
			response, err := decodeCensorshipError(body)
			if err != nil || status != 400 || response.Error.Code != "censorship_blocked" || response.Error.Role != "developer" {
				t.Fatalf("status=%d body=%s error=%v", status, body, err)
			}
			if upstream.arrivalCount() != 0 {
				t.Fatal("blocked Gemini response schema reached upstream")
			}
		})
	}
}

func TestHTTPInteractionsStepListBlockReturnsLocalError(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"model":"censorship-integration-model","input":[{"role":"user","steps":[{"content":"SECRET nested step"}]}]}`)
	status, _, response := postJSON(t, cpa.baseURL+"/v1beta/interactions", body)
	errorResponse, err := decodeCensorshipError(response)
	if err != nil || status != 400 || errorResponse.Error.Code != "censorship_blocked" || errorResponse.Error.Role != "user" {
		t.Fatalf("status=%d body=%s", status, response)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked Interactions step list reached upstream")
	}
}

func TestHTTPChatToolDescriptionBlockReturnsLocalError(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [SECRET]\nscope:\n  roles: [developer]\n")
	body := []byte(`{"model":"censorship-integration-model","messages":[{"role":"user","content":"plain"}],"tools":[{"type":"function","function":{"name":"safe","description":"SECRET description","parameters":{"type":"object"}}}]}`)
	status, _, response := postJSON(t, cpa.baseURL+"/v1/chat/completions", body)
	errorResponse, err := decodeCensorshipError(response)
	if err != nil || status != 400 || errorResponse.Error.Code != "censorship_blocked" || errorResponse.Error.Role != "developer" {
		t.Fatalf("status=%d body=%s", status, response)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("blocked OpenAI definition reached upstream")
	}
}

func TestHTTPResponsesTransformsStringInput(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: strip\nwords: [SECRET]\n")
	status, _, body := postJSON(t, cpa.baseURL+"/v1/responses", []byte(`{"model":"censorship-integration-model","input":"SECRET input"}`))
	if status != 200 {
		t.Fatalf("status=%d body=%s", status, body)
	}
	captured := upstream.lastRequest()
	if got := gjson.GetBytes(captured, "messages.0.content").String(); got != " input" {
		t.Fatalf("upstream input = %q, body = %s", got, captured)
	}
}

func TestHTTPResponsesTransformsStructuredInputText(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: strip\nwords: [SECRET]\n")
	body := []byte(`{"model":"censorship-integration-model","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET input"}]}]}`)
	status, _, response := postJSON(t, cpa.baseURL+"/v1/responses", body)
	if status != 200 {
		t.Fatalf("status=%d body=%s", status, response)
	}
	captured := upstream.lastRequest()
	if got := gjson.GetBytes(captured, "messages.0.content.0.text").String(); got != " input" {
		t.Fatalf("upstream input text = %q, body = %s", got, captured)
	}
}

func TestCapturedChatContentValidatorRejectsTransformedDecoy(t *testing.T) {
	captured := []byte(`{"messages":[{"content":"before SECRET after"}],"decoy":"before  after"}`)
	if err := validateCapturedChatContent(captured, "before SECRET after", "before  after"); err == nil {
		t.Fatal("validator accepted a transformed decoy while messages[0].content remained untransformed")
	}
}

func TestCapturedChatRequestValidatorRejectsChangedNonTargetField(t *testing.T) {
	disabled := []byte(`{"model":"censorship-integration-model","messages":[{"role":"user","content":"before SECRET after"}]}`)
	enabled := []byte(`{"model":"censorship-integration-model","messages":[{"role":"assistant","content":"before  after"}]}`)
	if err := validateCapturedChatRequest(enabled, disabled, "before SECRET after", "before  after"); err == nil {
		t.Fatal("validator accepted a changed non-target field")
	}
}

func validateCapturedChatContent(captured []byte, input, transformed string) error {
	got := gjson.GetBytes(captured, "messages.0.content").String()
	if got != transformed {
		return fmt.Errorf("upstream message content = %q, want %q; body = %s", got, transformed, captured)
	}
	return nil
}

func validateCapturedChatRequest(enabled, disabled []byte, input, transformed string) error {
	if err := validateCapturedChatContent(enabled, input, transformed); err != nil {
		return err
	}
	normalized, err := sjson.SetBytes(enabled, "messages.0.content", input)
	if err != nil {
		return fmt.Errorf("restore upstream message content: %w", err)
	}
	if !bytes.Equal(normalized, disabled) {
		return fmt.Errorf("non-target upstream request fields differ: enabled = %s, disabled = %s", enabled, disabled)
	}
	return nil
}

func TestHTTPAndSSEOutputTraceUnaffected(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("nonmatching/stream=%t", stream), func(t *testing.T) {
			disabledUpstream := newMockUpstream(t)
			enabledUpstream := newMockUpstream(t)
			disabled := startCPA(t, disabledUpstream.URL, false, "")
			enabled := startCPA(t, enabledUpstream.URL, true, "mode: strip\nwords: [NEVER-MATCH]\n")
			body := chatBody("plain input", stream)
			gotDisabled := captureHTTP11Trace(t, disabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			gotEnabled := captureHTTP11Trace(t, enabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			if !reflect.DeepEqual(gotEnabled, gotDisabled) {
				t.Fatalf("enabled trace = %#v, disabled trace = %#v", gotEnabled, gotDisabled)
			}
		})
	}

	for _, tc := range []struct {
		name, config, input, transformed string
	}{
		{name: "strip", config: "mode: strip\nwords: [SECRET]\n", input: "before SECRET after", transformed: "before  after"},
		{name: "obfs", config: "mode: obfs\nwords: [SECRET]\n", input: "before SECRET after", transformed: "before S​ECRET after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			disabledUpstream := newMockUpstream(t)
			enabledUpstream := newMockUpstream(t)
			disabled := startCPA(t, disabledUpstream.URL, false, "")
			enabled := startCPA(t, enabledUpstream.URL, true, tc.config)
			body := []byte(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":%q}],"stream":true,"decoy":%q}`, modelName, tc.input, tc.transformed))
			disabledTrace := captureHTTP11Trace(t, disabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			enabledTrace := captureHTTP11Trace(t, enabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			enabledRequest := enabledUpstream.lastRequest()
			disabledRequest := disabledUpstream.lastRequest()
			if err := validateCapturedChatRequest(enabledRequest, disabledRequest, tc.input, tc.transformed); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(enabledTrace, disabledTrace) {
				t.Fatalf("enabled trace = %#v, disabled trace = %#v", enabledTrace, disabledTrace)
			}
		})
	}
}

type functionalHTTPFixture struct {
	name, format, provider, endpoint, role, fragment, textPath string
	machine                                                    map[string]string
	carrier                                                    bool
	imageURL                                                   string
}

func functionalHTTPString(t *testing.T, text string) string {
	t.Helper()
	raw, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func functionalHTTPBody(t *testing.T, fixture functionalHTTPFixture, text string) []byte {
	t.Helper()
	if fixture.carrier {
		filePart := `{"type":"input_image","file_id":"SECRET file"}`
		if fixture.provider == "" && fixture.name != "legacy_function_raw" {
			// Chat consumer 保留 file-ID 对象的 JSON 文本，不调用文件服务。
			filePart = `{"type":"input_file","file_id":"SECRET file"}`
		}
		text = `[{"type":"text","text":` + functionalHTTPString(t, text) + `},{"type":"input_text","text":` + functionalHTTPString(t, text) + `},{"type":"output_text","text":` + functionalHTTPString(t, text) + `},{"type":"image_url","image_url":{"url":` + functionalHTTPString(t, fixture.imageURL) + `}},` + filePart + `,{"type":"unknown","text":"SECRET unknown"}]`
	}
	return functionalHTTPRawBody(fixture, functionalHTTPString(t, text))
}

func functionalHTTPRawBody(fixture functionalHTTPFixture, token string) []byte {
	return []byte(`{"model":"censorship-integration-model",` + fmt.Sprintf(fixture.fragment, token) + `}`)
}

func newFunctionalDirectUpstream(t *testing.T, provider string) *mockUpstream {
	t.Helper()
	capture := new(upstreamCapture)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		validPath := provider == "claude" && r.URL.Path == "/v1/messages" || provider == "codex" && strings.HasSuffix(r.URL.Path, "/responses") || provider == "gemini" && strings.HasSuffix(r.URL.Path, ":generateContent")
		if r.Method != http.MethodPost || !validPath {
			http.NotFound(w, r)
			return
		}
		capture.arrive()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !json.Valid(body) {
			http.Error(w, "invalid fixture request", http.StatusBadRequest)
			return
		}
		capture.record(body)
		w.Header().Set("Content-Type", "application/json")
		switch provider {
		case "claude":
			_, _ = io.WriteString(w, `{"id":"msg-fixture","type":"message","role":"assistant","model":"censorship-integration-model","content":[{"type":"text","text":"fixed upstream output"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		case "gemini":
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"fixed upstream output"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`)
		case "codex":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+`{"type":"response.completed","response":{"id":"resp-fixture","object":"response","model":"censorship-integration-model","status":"completed","output":[{"id":"msg-fixture","type":"message","role":"assistant","content":[{"type":"output_text","text":"fixed upstream output","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`+"\n\n")
		}
	}))
	t.Cleanup(server.Close)
	return &mockUpstream{Server: server, upstreamCapture: capture}
}

// startCPA 的现有配置只支持 OpenAI compatibility，直接协议测试复用其路径、就绪及清理接口。
func startFunctionalDirectCPA(t *testing.T, upstreamURL, provider, censorshipYAML string) *cpaInstance {
	t.Helper()
	paths, err := resolveCPAPaths(os.Getenv("CPA_INTEGRATION_BIN"), os.Getenv("CENSORSHIP_PLUGIN_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	configPath := filepath.Join(runDir, "config.yaml")
	base := fmt.Sprintf("host: 127.0.0.1\nport: %d\napi-keys: [%q]\ndisable-claude-cloak-mode: true\n%s-api-key:\n  - api-key: local-fixture-key\n    base-url: %q\n    models:\n      - name: %q\n        alias: %q\nplugins:\n  enabled: true\n  dir: %q\n  configs:\n", port, downstreamKey, provider, upstreamURL, modelName, modelName, paths.pluginDir)
	if err := os.WriteFile(configPath, []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	writePluginConfig(t, configPath, censorshipYAML)
	logPath := filepath.Join(runDir, "cpa.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(paths.binary, "--config", configPath, "--no-browser")
	cmd.Dir, cmd.Stdout, cmd.Stderr = runDir, logFile, logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatal(err)
	}
	cpa := &cpaInstance{cmd: cmd, config: configPath, baseURL: fmt.Sprintf("http://127.0.0.1:%d", port), waitDone: make(chan error, 1), logPath: logPath, logFile: logFile}
	go func() {
		waitErr := cmd.Wait()
		cpa.closeLogFile()
		cpa.waitDone <- waitErr
		close(cpa.waitDone)
	}()
	t.Cleanup(func() { stopCPA(t, cpa) })
	waitForCPA(t, cpa)
	return cpa
}

func runFunctionalHTTPMatrix(t *testing.T, fixtures []functionalHTTPFixture) {
	t.Helper()
	for _, fixture := range fixtures {
		for _, action := range []struct{ name, config, input, want string }{
			{"block", "words:\n  block: [SECRET]\n", "SECRET tail", ""},
			{"strip", "words:\n  strip: [SECRET]\n", "SECRET tail", " tail"},
			{"obfs", "words:\n  obfs: [SECRET]\n", "SECRET tail", "S​ECRET tail"},
			{"filter_bypass", "words:\n  block: [SECRET]\nfilter_mode: include\nfilter:\n  models: [never-match]\n", "SECRET tail", "SECRET tail"},
			{"cancellation", "words:\n  strip: ['​']\n  obfs: [ab]\n", "a​b tail", "a​b tail"},
		} {
			t.Run(fixture.name+"/"+action.name, func(t *testing.T) {
				config := action.config + "scope:\n  roles: [" + fixture.role + "]\n"
				if fixture.carrier {
					config = strings.Replace(config, "[SECRET]", "[SECRET, iVBOR]", 1)
				}
				var upstream *mockUpstream
				var cpa *cpaInstance
				if fixture.provider == "" {
					upstream = newMockUpstream(t)
					cpa = startCPA(t, upstream.URL, true, config)
				} else {
					upstream = newFunctionalDirectUpstream(t, fixture.provider)
					cpa = startFunctionalDirectCPA(t, upstream.URL, fixture.provider, config)
				}
				defer func() {
					t.Logf("GOOS=%s consumer=%s CPA log\n%s", runtime.GOOS, fixture.provider, readCPALog(cpa.logPath))
				}()
				status, header, response := postJSON(t, cpa.baseURL+fixture.endpoint, functionalHTTPBody(t, fixture, action.input))
				if action.name == "block" {
					errorResponse, err := decodeCensorshipError(response)
					if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || errorResponse.Error.Code != "censorship_blocked" || errorResponse.Error.Term != "SECRET" || errorResponse.Error.Role != fixture.role || upstream.arrivalCount() != 0 || upstream.requestCount() != 0 {
						t.Fatalf("block status=%d arrivals=%d requests=%d body=%s error=%v\n%s", status, upstream.arrivalCount(), upstream.requestCount(), response, err, readCPALog(cpa.logPath))
					}
					return
				}
				if status != 200 || upstream.arrivalCount() != 1 || upstream.requestCount() != 1 {
					t.Fatalf("status=%d arrivals=%d body=%s\n%s", status, upstream.arrivalCount(), response, readCPALog(cpa.logPath))
				}
				captured := upstream.lastRequest()
				if fixture.carrier {
					requireFunctionalCarrier(t, fixture, captured, action.want)
				} else if got := gjson.GetBytes(captured, fixture.textPath).String(); got != action.want {
					t.Fatalf("upstream %s=%q, want %q; body=%s", fixture.textPath, got, action.want, captured)
				}
				for path, wantRaw := range fixture.machine {
					if got := gjson.GetBytes(captured, path).Raw; got != wantRaw {
						t.Errorf("machine %s=%s, want raw %s", path, got, wantRaw)
					}
				}
			})
		}
	}
}

func functionalHTTPFlatCustomFixtures() []functionalHTTPFixture {
	return []functionalHTTPFixture{
		{name: "Chat_flat_custom", format: "openai", provider: "codex", endpoint: "/v1/chat/completions", role: "developer", fragment: `"messages":[{"role":"user","content":"safe"}],"tools":[{"type":"custom","name":"SECRET_tool","description":%s,"format":{"type":"grammar","syntax":"lark","definition":"start: \"SECRET\""},"id":"SECRET id"}]`, textPath: "tools.0.description", machine: map[string]string{"tools.0.type": `"custom"`, "tools.0.name": `"SECRET_tool"`, "tools.0.format.type": `"grammar"`, "tools.0.format.syntax": `"lark"`, "tools.0.format.definition": `"start: \"SECRET\""`, "tools.0.id": `"SECRET id"`}},
	}
}

func TestHTTPFunctionalChatFlatCustomToCodex(t *testing.T) {
	fixtures := functionalHTTPFlatCustomFixtures()
	runFunctionalHTTPMatrix(t, fixtures)
	fixture := fixtures[0]
	for _, tc := range []struct {
		name, config, input, want string
		blocked                   bool
	}{
		{"default_scope", "words:\n  block: [SECRET]\n", "SECRET tail", "", true},
		{"developer_disabled", "words:\n  block: [SECRET]\nscope:\n  roles: [user]\n", "SECRET tail", "SECRET tail", false},
		{"no_match", "words:\n  block: [SECRET]\n", "safe tail", "safe tail", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := newFunctionalDirectUpstream(t, fixture.provider)
			cpa := startFunctionalDirectCPA(t, upstream.URL, fixture.provider, tc.config)
			defer func() { t.Logf("Chat->Codex CPA log\n%s", readCPALog(cpa.logPath)) }()
			status, header, response := postJSON(t, cpa.baseURL+fixture.endpoint, functionalHTTPBody(t, fixture, tc.input))
			if tc.blocked {
				decoded, err := decodeCensorshipError(response)
				if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != "SECRET" || decoded.Error.Role != "developer" || upstream.arrivalCount() != 0 || upstream.requestCount() != 0 {
					t.Fatalf("block status=%d arrivals=%d requests=%d body=%s error=%v", status, upstream.arrivalCount(), upstream.requestCount(), response, err)
				}
				return
			}
			if status != 200 || upstream.arrivalCount() != 1 || upstream.requestCount() != 1 {
				t.Fatalf("status=%d arrivals=%d requests=%d body=%s", status, upstream.arrivalCount(), upstream.requestCount(), response)
			}
			captured := upstream.lastRequest()
			value := gjson.GetBytes(captured, fixture.textPath)
			if value.Type != gjson.String || value.String() != tc.want {
				t.Fatalf("upstream description=%s, want %q; body=%s", value.Raw, tc.want, captured)
			}
			for path, want := range fixture.machine {
				if got := gjson.GetBytes(captured, path).Raw; got != want {
					t.Errorf("machine %s=%s, want %s", path, got, want)
				}
			}
		})
	}
}

func TestHTTPFunctionalClaudeMCPAndHistory(t *testing.T) {
	runFunctionalHTTPMatrix(t, functionalHTTPClaudeMCPFixtures())
}

func functionalHTTPClaudeMCPFixtures() []functionalHTTPFixture {
	return []functionalHTTPFixture{
		{name: "MCP_scalar", format: "claude", provider: "claude", endpoint: "/v1/messages", role: "tool", fragment: `"max_tokens":16,"messages":[{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"SECRET id","content":%s,"is_error":false}]},{"role":"user","content":"safe"}]`, textPath: "messages.0.content.0.content", machine: map[string]string{"messages.0.content.0.tool_use_id": `"SECRET id"`, "messages.0.content.0.is_error": "false"}},
		{name: "MCP_typed", format: "claude", provider: "claude", endpoint: "/v1/messages", role: "tool", fragment: `"max_tokens":16,"messages":[{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"SECRET id","content":[{"type":"text","text":%s},{"type":"image","source":{"type":"url","url":"https://example.test/SECRET.png"}},{"type":"unknown","text":"SECRET unknown"}]}]},{"role":"user","content":"safe"}]`, textPath: "messages.0.content.0.content.0.text", machine: map[string]string{"messages.0.content.0.tool_use_id": `"SECRET id"`, "messages.0.content.0.content.1.source.url": `"https://example.test/SECRET.png"`, "messages.0.content.0.content.2.text": `"SECRET unknown"`}},
		{name: "history_scalar", format: "claude", provider: "claude", endpoint: "/v1/messages", role: "assistant", fragment: `"max_tokens":16,"messages":[{"role":"assistant","content":%s},{"role":"user","content":"safe"}]`, textPath: "messages.0.content", machine: map[string]string{"messages.1.content.0.text": `"safe"`}},
	}
}

func TestHTTPFunctionalClaudeDocumentTitle(t *testing.T) {
	runFunctionalHTTPMatrix(t, functionalHTTPClaudeTitleFixtures())
}

func functionalHTTPClaudeTitleFixtures() []functionalHTTPFixture {
	return []functionalHTTPFixture{
		{name: "direct", format: "claude", provider: "claude", endpoint: "/v1/messages", role: "user", fragment: `"max_tokens":16,"messages":[{"role":"user","content":[{"type":"document","title":%s,"source":{"type":"text","data":"safe source"},"citations":{"enabled":true}}]}]`, textPath: "messages.0.content.0.title", machine: map[string]string{"messages.0.content.0.source.data": `"safe source"`, "messages.0.content.0.citations.enabled": "true"}},
		{name: "nested", format: "claude", provider: "claude", endpoint: "/v1/messages", role: "tool", fragment: `"max_tokens":16,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"SECRET id","content":[{"type":"document","title":%s,"source":{"type":"text","data":"safe source"}}]}]}]`, textPath: "messages.0.content.0.content.0.title", machine: map[string]string{"messages.0.content.0.tool_use_id": `"SECRET id"`, "messages.0.content.0.content.0.source.data": `"safe source"`}},
	}
}

func functionalHTTPSchemaFixtures() []functionalHTTPFixture {
	fixtures := []functionalHTTPFixture{}
	for _, property := range []bool{false, true} {
		schema := `{"type":"object","description":%s,"properties":{"SECRET key":{"type":"string","enum":["SECRET enum"],"default":"SECRET default"}}}`
		name, suffix := "root", "description"
		if property {
			schema = `{"type":"object","properties":{"SECRET key":{"type":"string","description":%s,"enum":["SECRET enum"],"default":"SECRET default"}}}`
			name, suffix = "property", "properties.SECRET key.description"
		}
		fixtures = append(fixtures,
			functionalHTTPFixture{name: "Chat_" + name, format: "openai", endpoint: "/v1/chat/completions", role: "developer", fragment: `"messages":[{"role":"user","content":"safe"}],"tools":[{"type":"function","function":{"name":"SECRET name","parametersJsonSchema":` + schema + `}}]`, textPath: "tools.0.function.parametersJsonSchema." + suffix, machine: map[string]string{"tools.0.function.name": `"SECRET name"`, "tools.0.function.parametersJsonSchema.properties.SECRET key.enum.0": `"SECRET enum"`}},
			functionalHTTPFixture{name: "Responses_" + name, format: "openai-response", provider: "codex", endpoint: "/v1/responses", role: "developer", fragment: `"input":"safe","text":{"format":{"type":"json_schema","name":"SECRET name","json_schema":{"schema":` + schema + `}}}`, textPath: "text.format.json_schema.schema." + suffix, machine: map[string]string{"text.format.name": `"SECRET name"`, "text.format.json_schema.schema.properties.SECRET key.enum.0": `"SECRET enum"`}},
		)
		for _, alias := range []string{"response_schema", "_responseJsonSchema", "response_json_schema", "response_json_schema_ordered"} {
			prefix := "generationConfig." + alias
			fixtures = append(fixtures, functionalHTTPFixture{name: "Gemini_" + alias + "_" + name, format: "gemini", provider: "gemini", endpoint: "/v1beta/models/censorship-integration-model:generateContent", role: "developer", fragment: `"contents":[{"role":"user","parts":[{"text":"safe"}]}],"generationConfig":{"responseMimeType":"application/json","` + alias + `":` + schema + `}`, textPath: prefix + "." + suffix, machine: map[string]string{prefix + ".properties.SECRET key.enum.0": `"SECRET enum"`, prefix + ".properties.SECRET key.default": `"SECRET default"`}})
		}
	}
	return fixtures
}

func TestHTTPFunctionalSchemaAliases(t *testing.T) {
	runFunctionalHTTPMatrix(t, functionalHTTPSchemaFixtures())
}

func functionalHTTPDecodeFixtures() []functionalHTTPFixture {
	return []functionalHTTPFixture{
		{name: "Chat", format: "openai", endpoint: "/v1/chat/completions", role: "user", fragment: `"messages":[{"role":"user","content":%s}]`, textPath: "messages.0.content"},
		{name: "Responses", format: "openai-response", endpoint: "/v1/responses", role: "user", fragment: `"input":%s`, textPath: "messages.0.content"},
		{name: "Claude", format: "claude", provider: "claude", endpoint: "/v1/messages", role: "user", fragment: `"max_tokens":16,"messages":[{"role":"user","content":%s}]`, textPath: "messages.0.content.0.text"},
		{name: "Gemini", format: "gemini", provider: "gemini", endpoint: "/v1beta/models/censorship-integration-model:generateContent", role: "user", fragment: `"contents":[{"role":"user","parts":[{"text":%s}]}]`, textPath: "contents.0.parts.0.text"},
		{name: "Interactions", format: "interactions", endpoint: "/v1beta/interactions", role: "user", fragment: `"input":%s`, textPath: "messages.0.content"},
	}
}

func TestHTTPFunctionalDecodeAllFormats(t *testing.T) {
	runFunctionalHTTPMatrix(t, functionalHTTPDecodeFixtures())
}

type functionalHTTPSurrogateCase struct {
	name, mode, term, token, want string
}

func functionalHTTPSurrogateCases(t *testing.T) []functionalHTTPSurrogateCase {
	t.Helper()
	const escape = `\u`
	var cases []functionalHTTPSurrogateCase
	for _, surrogate := range []string{"d800", "dc00"} {
		token := `"` + escape + surrogate + escape + `0065xample note"`
		var original string
		if err := json.Unmarshal([]byte(token), &original); err != nil {
			t.Fatal(err)
		}
		for _, action := range []struct{ name, mode, term, replacement string }{
			{"block", "block", "example", ""},
			{"strip", "strip", "example", ""},
			{"obfs", "obfs", "example", "e​xample"},
			{"strip_other", "strip", "note", ""},
			{"obfs_other", "obfs", "note", "n​ote"},
			{"no_match", "strip", "MISS", ""},
		} {
			cases = append(cases, functionalHTTPSurrogateCase{
				name: "surrogate_" + surrogate + "_" + action.name, mode: action.mode, term: action.term,
				token: token, want: strings.ReplaceAll(original, action.term, action.replacement),
			})
		}
	}
	return cases
}

func TestHTTPFunctionalRawSurrogateAllFormats(t *testing.T) {
	for _, fixture := range functionalHTTPDecodeFixtures() {
		for _, action := range functionalHTTPSurrogateCases(t) {
			// 原始 no-op bytes 在 RPC/native 边界检查；HTTP 检查插件改写后的文本。
			if strings.HasSuffix(action.name, "_no_match") {
				continue
			}
			t.Run(fixture.name+"/"+action.name, func(t *testing.T) {
				config := "words:\n  " + action.mode + ": [" + action.term + "]\nscope:\n  roles: [user]\n"
				var upstream *mockUpstream
				var cpa *cpaInstance
				if fixture.provider == "" {
					upstream = newMockUpstream(t)
					cpa = startCPA(t, upstream.URL, true, config)
				} else {
					upstream = newFunctionalDirectUpstream(t, fixture.provider)
					cpa = startFunctionalDirectCPA(t, upstream.URL, fixture.provider, config)
				}
				defer func() {
					t.Logf("GOOS=%s raw token=%s CPA log\n%s", runtime.GOOS, action.token, readCPALog(cpa.logPath))
				}()
				status, header, response := postJSON(t, cpa.baseURL+fixture.endpoint, functionalHTTPRawBody(fixture, action.token))
				if action.mode == "block" {
					decoded, err := decodeCensorshipError(response)
					if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != action.term || decoded.Error.Role != "user" || upstream.arrivalCount() != 0 {
						t.Fatalf("block status=%d arrivals=%d body=%s error=%v", status, upstream.arrivalCount(), response, err)
					}
					return
				}
				if status != 200 || upstream.arrivalCount() != 1 || upstream.requestCount() != 1 {
					t.Fatalf("status=%d arrivals=%d body=%s", status, upstream.arrivalCount(), response)
				}
				var got string
				if err := json.Unmarshal([]byte(gjson.GetBytes(upstream.lastRequest(), fixture.textPath).Raw), &got); err != nil || got != action.want {
					t.Fatalf("upstream text=%q want=%q error=%v body=%s", got, action.want, err, upstream.lastRequest())
				}
			})
		}
	}
}

func functionalHTTPOutputFixtures() []functionalHTTPFixture {
	return []functionalHTTPFixture{
		{name: "function_scalar", format: "openai-response", provider: "codex", endpoint: "/v1/responses", role: "tool", fragment: `"input":[{"type":"function_call_output","call_id":"SECRET call","output":%s}]`, textPath: "input.0.output", machine: map[string]string{"input.0.call_id": `"SECRET call"`}},
		{name: "function_typed", format: "openai-response", provider: "codex", endpoint: "/v1/responses", role: "tool", fragment: `"input":[{"type":"function_call_output","call_id":"SECRET call","output":[{"type":"input_text","text":%s}]}]`, textPath: "input.0.output.0.text", machine: map[string]string{"input.0.call_id": `"SECRET call"`}},
		{name: "shell_stdout", format: "openai-response", provider: "codex", endpoint: "/v1/responses", role: "tool", fragment: `"input":[{"type":"shell_call_output","call_id":"SECRET call","output":[{"stdout":%s,"stderr":"safe","exit_code":1}]}]`, textPath: "input.0.output.0.stdout", machine: map[string]string{"input.0.output.0.stderr": `"safe"`, "input.0.output.0.exit_code": "1"}},
		{name: "shell_stderr", format: "openai-response", provider: "codex", endpoint: "/v1/responses", role: "tool", fragment: `"input":[{"type":"shell_call_output","call_id":"SECRET call","output":[{"stdout":"safe","stderr":%s,"exit_code":1}]}]`, textPath: "input.0.output.0.stderr", machine: map[string]string{"input.0.output.0.stdout": `"safe"`, "input.0.output.0.exit_code": "1"}},
		{name: "apply_patch", format: "openai-response", provider: "codex", endpoint: "/v1/responses", role: "tool", fragment: `"input":[{"type":"apply_patch_call_output","call_id":"SECRET call","output":%s}]`, textPath: "input.0.output", machine: map[string]string{"input.0.call_id": `"SECRET call"`}},
		{name: "program_result", format: "openai-response", provider: "codex", endpoint: "/v1/responses", role: "tool", fragment: `"input":[{"type":"program_output","result":%s,"id":"SECRET id"}]`, textPath: "input.0.result", machine: map[string]string{"input.0.id": `"SECRET id"`}},
	}
}

func TestHTTPFunctionalResponsesOutputs(t *testing.T) {
	runFunctionalHTTPMatrix(t, functionalHTTPOutputFixtures())
}

func functionalHTTPCarrierFixtures(t *testing.T) []functionalHTTPFixture {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	imageURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	return []functionalHTTPFixture{
		{name: "Chat_tool_to_Codex", format: "openai", provider: "codex", endpoint: "/v1/chat/completions", role: "tool", fragment: `"messages":[{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"SECRET_lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call-1","content":%s}]`, textPath: `input.#(type=="function_call_output").output`, machine: map[string]string{`input.#(type=="function_call_output").call_id`: `"call-1"`, `input.#(type=="function_call").name`: `"SECRET_lookup"`}, carrier: true, imageURL: imageURL},
		{name: "Responses_function_to_Chat", format: "openai-response", endpoint: "/v1/responses", role: "tool", fragment: `"input":[{"type":"function_call_output","call_id":"SECRET call","output":%s}]`, textPath: `messages.#(role=="tool").content`, machine: map[string]string{`messages.#(role=="tool").tool_call_id`: `"SECRET call"`}, carrier: true, imageURL: imageURL},
		{name: "Responses_custom_to_Chat", format: "openai-response", endpoint: "/v1/responses", role: "tool", fragment: `"input":[{"type":"custom_tool_call_output","call_id":"SECRET call","output":%s}]`, textPath: `messages.#(role=="tool").content`, machine: map[string]string{`messages.#(role=="tool").tool_call_id`: `"SECRET call"`}, carrier: true, imageURL: imageURL},
		{name: "legacy_function_raw", format: "openai", endpoint: "/v1/chat/completions", role: "tool", fragment: `"messages":[{"role":"function","name":"SECRET name","content":%s}]`, textPath: "messages.0.content", machine: map[string]string{"messages.0.role": `"function"`, "messages.0.name": `"SECRET name"`}, carrier: true, imageURL: imageURL},
	}
}

func requireFunctionalCarrier(t *testing.T, fixture functionalHTTPFixture, captured []byte, wantText string) {
	t.Helper()
	parts := gjson.GetBytes(captured, fixture.textPath)
	if fixture.name == "legacy_function_raw" {
		if parts.Type != gjson.String {
			t.Fatalf("legacy function scalar changed to %s", parts.Raw)
		}
		parts = gjson.Parse(parts.String())
	}
	if !parts.IsArray() || parts.Get("#").Int() != 6 {
		t.Fatalf("carrier consumer %s=%s", fixture.textPath, parts.Raw)
	}
	for _, index := range []string{"0", "1", "2"} {
		if got := parts.Get(index + ".text").String(); got != wantText {
			t.Errorf("carrier text[%s]=%q, want %q", index, got, wantText)
		}
	}
	imagePath := "3.image_url.url"
	fileID := parts.Get("4.file_id").String()
	unknown := parts.Get("5.text").String()
	if fixture.provider == "codex" {
		imagePath = "3.image_url"
		if parts.Get("0.type").String() != "input_text" || parts.Get("3.type").String() != "input_image" {
			t.Fatalf("Codex consumer types=%s", parts.Raw)
		}
	} else if fixture.name != "legacy_function_raw" {
		if parts.Get("0.type").String() != "text" || parts.Get("3.type").String() != "image_url" || parts.Get("4.text").String() != `{"type":"input_file","file_id":"SECRET file"}` {
			t.Fatalf("Chat consumer types/file-ID=%s", parts.Raw)
		}
		fileID = gjson.Get(parts.Get("4.text").String(), "file_id").String()
	}
	if fixture.name != "legacy_function_raw" && unknown != `{"type":"unknown","text":"SECRET unknown"}` {
		t.Fatalf("consumer unknown token=%q", unknown)
	}
	url := parts.Get(imagePath).String()
	if url != fixture.imageURL || fileID != "SECRET file" || fixture.name == "legacy_function_raw" && unknown != "SECRET unknown" {
		t.Fatalf("carrier image/file/unknown changed: %s", parts.Raw)
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil || decoded.Bounds() != image.Rect(0, 0, 2, 2) {
		t.Fatalf("consumer PNG decode error=%v", err)
	}
}

func TestHTTPFunctionalEncodedToolConsumers(t *testing.T) {
	runFunctionalHTTPMatrix(t, functionalHTTPCarrierFixtures(t))
}

func TestHTTPFunctionalInvalidBoundaries(t *testing.T) {
	fixtures := append(functionalHTTPClaudeTitleFixtures(), functionalHTTPOutputFixtures()...)
	for _, fixture := range fixtures {
		limit := 10485760
		if fixture.format == "claude" {
			limit = 500
		}
		for _, finalLength := range []int{limit, limit + 1} {
			t.Run(fmt.Sprintf("%s/final_%d", fixture.name, finalLength), func(t *testing.T) {
				upstream := newFunctionalDirectUpstream(t, fixture.provider)
				cpa := startFunctionalDirectCPA(t, upstream.URL, fixture.provider, "words:\n  obfs: [ab]\nscope:\n  roles: ["+fixture.role+"]\n")
				text := strings.Repeat("x", finalLength-3) + "ab"
				status, header, response := postJSON(t, cpa.baseURL+fixture.endpoint, functionalHTTPBody(t, fixture, text))
				if finalLength > limit {
					decoded, err := decodeCensorshipError(response)
					if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_invalid_request" || upstream.arrivalCount() != 0 {
						t.Fatalf("invalid status=%d arrivals=%d body=%s error=%v", status, upstream.arrivalCount(), response, err)
					}
				} else {
					want := strings.Repeat("x", finalLength-3) + "a​b"
					got := gjson.GetBytes(upstream.lastRequest(), fixture.textPath).String()
					if status != 200 || upstream.arrivalCount() != 1 || got != want {
						t.Fatalf("at-limit status=%d arrivals=%d text bytes=%d want=%d response=%s", status, upstream.arrivalCount(), len(got), len(want), response)
					}
				}
			})
		}
	}
	emptyFixtures := append(functionalHTTPClaudeMCPFixtures(), functionalHTTPClaudeTitleFixtures()...)
	emptyFixtures = append(emptyFixtures, functionalHTTPFixture{name: "last_scalar", format: "claude", provider: "claude", endpoint: "/v1/messages", role: "assistant", fragment: `"max_tokens":16,"messages":[{"role":"user","content":"safe"},{"role":"assistant","content":%s}]`, textPath: "messages.1.content.0.text"})
	for _, fixture := range emptyFixtures {
		t.Run(fixture.name+"/full_strip", func(t *testing.T) {
			upstream := newFunctionalDirectUpstream(t, "claude")
			cpa := startFunctionalDirectCPA(t, upstream.URL, "claude", "words:\n  strip: [SECRET]\nscope:\n  roles: ["+fixture.role+"]\n")
			status, header, response := postJSON(t, cpa.baseURL+fixture.endpoint, functionalHTTPBody(t, fixture, "SECRET"))
			if fixture.name == "MCP_scalar" || fixture.name == "last_scalar" {
				value := gjson.GetBytes(upstream.lastRequest(), fixture.textPath)
				if status != 200 || upstream.arrivalCount() != 1 || !value.Exists() || value.Type != gjson.String || value.String() != "" {
					t.Fatalf("allowed empty %s status=%d upstream=%s response=%s", fixture.name, status, upstream.lastRequest(), response)
				}
				return
			}
			decoded, err := decodeCensorshipError(response)
			if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_invalid_request" || upstream.arrivalCount() != 0 {
				t.Fatalf("nonempty status=%d arrivals=%d body=%s error=%v", status, upstream.arrivalCount(), response, err)
			}
		})
	}
}

func TestHTTPFunctionalNativeChatFlatCustomGates(t *testing.T) {
	for _, phase := range []string{"BeforeAuth", "AfterAuth"} {
		for _, tc := range []struct {
			name, config, input, want, fragment, term, role string
			noOp                                            bool
		}{
			{name: "default_block", config: "words:\n  block: [SECRET]\n", input: "SECRET tail", term: "SECRET", role: "developer"},
			{name: "default_strip", config: "words:\n  strip: [SECRET]\n", input: "SECRET tail", want: " tail"},
			{name: "default_obfs", config: "words:\n  obfs: [SECRET]\n", input: "SECRET tail", want: "S​ECRET tail"},
			{name: "developer_disabled", config: "words:\n  block: [SECRET]\nscope:\n  roles: [user]\n", input: "SECRET tail", noOp: true},
			{name: "format_disabled", config: "words:\n  block: [SECRET]\nscope:\n  formats: []\n", input: "SECRET tail", noOp: true},
			{name: "no_match", config: "words:\n  strip: [SECRET]\n", input: "safe tail", noOp: true},
			{name: "machine_only", config: "words:\n  block: [SECRET]\n", input: "safe", noOp: true},
			{name: "full_strip", config: "words:\n  strip: [SECRET]\n", input: "SECRET", want: ""},
			{name: "cascade_once", config: "words:\n  strip: [X]\n  obfs: [ab, ab]\n", input: "XaXb", want: "a​b"},
			{name: "rule_major", config: "words:\n  block: [LATE, EARLY]\n", input: "EARLY", fragment: `"tools":[{"type":"custom","name":"safe","description":%s}],"messages":[{"role":"user","content":"LATE"}]`, term: "LATE", role: "user"},
			{name: "earliest_user", config: "words:\n  block: [SECRET]\n", input: "SECRET tail", fragment: `"messages":[{"role":"user","content":"SECRET first"}],"tools":[{"type":"custom","name":"safe","description":%s}]`, term: "SECRET", role: "user"},
			{name: "earliest_developer", config: "words:\n  block: [SECRET]\n", input: "SECRET tail", fragment: `"tools":[{"type":"custom","name":"safe","description":%s}],"messages":[{"role":"user","content":"SECRET later"}]`, term: "SECRET", role: "developer"},
			{name: "no_cross_node", config: "words:\n  block: [SECRET]\n", input: "SEC", fragment: `"tools":[{"type":"custom","name":"safe","description":%s,"custom":{"description":"RET"}}]`, noOp: true},
		} {
			t.Run(phase+"/"+tc.name, func(t *testing.T) {
				fixture := functionalHTTPFlatCustomFixtures()[0]
				if tc.fragment != "" {
					fixture.fragment = tc.fragment
				}
				configText := tc.config
				if phase == "AfterAuth" {
					configText += "filter_mode: include\nfilter:\n  models: [target-model]\n"
				}
				indented := "      " + strings.ReplaceAll(strings.TrimSuffix(configText, "\n"), "\n", "\n      ") + "\n"
				withDynamicABIHost(t, indented, func(before, after func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse) {
					body := functionalHTTPBody(t, fixture, tc.input)
					headers := http.Header{"Content-Encoding": {"identity"}, "Content-Length": {"123"}, "Transfer-Encoding": {"chunked"}, "X-Preserved": {"SECRET header"}}
					request := pluginapi.RequestInterceptRequest{SourceFormat: fixture.format, Model: "target-model", RequestedModel: "requested-decoy", Metadata: map[string]any{executor.SelectedAuthMetadataKey: "auth-1", "preserved": "SECRET"}, Body: bytes.Clone(body), Headers: headers.Clone()}
					snapshot, err := json.Marshal(request)
					if err != nil {
						t.Fatal(err)
					}
					unchanged := pluginapi.RequestInterceptResponse{Body: body, Headers: headers}
					call, inactive := before, after
					if phase == "AfterAuth" {
						call, inactive = after, before
					}
					if got := inactive(request); !reflect.DeepEqual(got, unchanged) {
						t.Fatalf("inactive phase=%#v", got)
					}
					response := call(request)
					finalRequest, err := json.Marshal(request)
					if err != nil || !bytes.Equal(snapshot, finalRequest) {
						t.Fatalf("native input/metadata/headers changed, error=%v", err)
					}
					if tc.term != "" {
						decoded, err := decodeCensorshipError(response.ResponseBody)
						if err != nil || !response.Terminate || response.StatusCode != 400 || !bytes.Equal(response.Body, body) || !reflect.DeepEqual(response.Headers, headers) || response.ClearHeaders != nil || response.ResponseHeaders.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != tc.term || decoded.Error.Role != tc.role {
							t.Fatalf("native block=%#v error=%v", response, err)
						}
					} else if tc.noOp {
						if !reflect.DeepEqual(response, unchanged) {
							t.Fatalf("native no-op=%#v", response)
						}
					} else if !reflect.DeepEqual(response, pluginapi.RequestInterceptResponse{Body: functionalHTTPBody(t, fixture, tc.want), Headers: http.Header{"X-Preserved": {"SECRET header"}}}) {
						t.Fatalf("native rewrite=%#v", response)
					}
				})
			})
		}
	}
}

func TestHTTPFunctionalNativeRegistrationAndPhases(t *testing.T) {
	pluginDir := os.Getenv("CENSORSHIP_PLUGIN_DIR")
	if pluginDir == "" {
		t.Fatal("CENSORSHIP_PLUGIN_DIR is required")
	}
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(fmt.Sprintf("plugins:\n  enabled: true\n  dir: %q\n  configs:\n    censorship:\n      enabled: true\n      words: {block: [SECRET]}\n", pluginDir)), &cfg); err != nil {
		t.Fatal(err)
	}
	host := pluginhost.New()
	host.ApplyConfig(context.Background(), &cfg)
	t.Cleanup(host.ShutdownAll)
	found := false
	for _, info := range host.RegisteredPlugins() {
		if info.ID != "censorship" {
			continue
		}
		found = true
		if info.Metadata.Name != "censorship" || info.Metadata.Version != "0.0.0-dev" || info.Metadata.Author != "DoingDog" || info.SupportsOAuth || len(info.Menus) != 0 {
			t.Fatalf("registration=%#v", info)
		}
		wantNames := []string{"ignore_case", "words", "scope", "obfs", "filter_mode", "filter_logic", "filter"}
		wantTypes := []pluginapi.ConfigFieldType{pluginapi.ConfigFieldTypeBoolean, pluginapi.ConfigFieldTypeObject, pluginapi.ConfigFieldTypeObject, pluginapi.ConfigFieldTypeObject, pluginapi.ConfigFieldTypeEnum, pluginapi.ConfigFieldTypeEnum, pluginapi.ConfigFieldTypeObject}
		if len(info.Metadata.ConfigFields) != len(wantNames) {
			t.Fatalf("ConfigFields=%#v", info.Metadata.ConfigFields)
		}
		for i, field := range info.Metadata.ConfigFields {
			if field.Name != wantNames[i] || field.Type != wantTypes[i] {
				t.Fatalf("ConfigFields[%d]=%#v", i, field)
			}
		}
		raw, err := json.Marshal(info)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("GOOS=%s native registered plugin=%s", runtime.GOOS, raw)
	}
	if !found || !host.PluginRegistered("censorship") {
		t.Fatal("native censorship registration missing")
	}
	fixtures := append(functionalHTTPFlatCustomFixtures(), functionalHTTPSchemaFixtures()...)
	fixtures = append(fixtures, functionalHTTPClaudeMCPFixtures()...)
	fixtures = append(fixtures, functionalHTTPClaudeTitleFixtures()...)
	fixtures = append(fixtures, functionalHTTPOutputFixtures()...)
	fixtures = append(fixtures, functionalHTTPDecodeFixtures()...)
	fixtures = append(fixtures, functionalHTTPCarrierFixtures(t)...)
	for _, fixture := range fixtures {
		actions := []functionalHTTPSurrogateCase{
			{name: "block", mode: "block", term: "SECRET"},
			{name: "strip", mode: "strip", term: "SECRET", want: " tail"},
			{name: "obfs", mode: "obfs", term: "SECRET", want: "S​ECRET tail"},
			{name: "cancellation", want: "a​b tail"},
		}
		switch fixture.name {
		case "Chat", "Responses", "Claude", "Gemini", "Interactions":
			actions = append(actions, functionalHTTPSurrogateCases(t)...)
		}
		for _, phase := range []string{"BeforeAuth", "AfterAuth"} {
			for _, action := range actions {
				t.Run(fixture.name+"/"+phase+"/"+action.name, func(t *testing.T) {
					configText := "words:\n  " + action.mode + ": [" + action.term + "]\nscope:\n  roles: [" + fixture.role + "]\n"
					input := "SECRET tail"
					if action.name == "cancellation" {
						input = action.want
						configText = "words:\n  strip: ['​']\n  obfs: [ab]\nscope:\n  roles: [" + fixture.role + "]\n"
					}
					if phase == "AfterAuth" {
						configText += "filter_mode: include\nfilter:\n  models: [target-model]\n"
					}
					indented := "      " + strings.ReplaceAll(strings.TrimSuffix(configText, "\n"), "\n", "\n      ") + "\n"
					withDynamicABIHost(t, indented, func(before, after func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse) {
						body := functionalHTTPBody(t, fixture, input)
						if action.token != "" {
							body = functionalHTTPRawBody(fixture, action.token)
						}
						metadata := map[string]any{executor.SelectedAuthMetadataKey: "auth-1", "preserved": "SECRET", "source_model": "source-decoy"}
						headers := http.Header{"Content-Encoding": {"identity"}, "Content-Length": {"123"}, "Transfer-Encoding": {"chunked"}, "X-Preserved": {"SECRET header"}}
						request := pluginapi.RequestInterceptRequest{RequestID: "functional-native", SourceFormat: fixture.format, Model: "target-model", RequestedModel: "requested-decoy", Metadata: metadata, Body: bytes.Clone(body), Headers: headers.Clone()}
						snapshot, err := json.Marshal(request)
						if err != nil {
							t.Fatal(err)
						}
						// Host 聚合原请求并应用 ClearHeaders；插件原始响应在 RPC 测试中精确断言。
						unchanged := pluginapi.RequestInterceptResponse{Body: body, Headers: headers}
						call := before
						if phase == "AfterAuth" {
							noOp := before(request)
							if !reflect.DeepEqual(noOp, unchanged) {
								t.Fatalf("model-bearing BeforeAuth=%#v", noOp)
							}
							call = after
							unselected := request
							unselected.Metadata = map[string]any{"source_model": "target-model"}
							if got := after(unselected); !reflect.DeepEqual(got, unchanged) {
								t.Fatalf("unselected AfterAuth=%#v", got)
							}
							miss := request
							miss.Model, miss.RequestedModel = "model-decoy", "target-model"
							if got := after(miss); !reflect.DeepEqual(got, unchanged) {
								t.Fatalf("filter miss=%#v", got)
							}
						}
						response := call(request)
						finalRequest, err := json.Marshal(request)
						if err != nil || !bytes.Equal(snapshot, finalRequest) {
							t.Fatalf("native request input/metadata/headers changed, error=%v", err)
						}
						if action.mode == "block" {
							decoded, err := decodeCensorshipError(response.ResponseBody)
							if err != nil || !response.Terminate || response.StatusCode != 400 || !bytes.Equal(response.Body, body) || response.ClearHeaders != nil || !reflect.DeepEqual(response.Headers, headers) || response.ResponseHeaders.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != action.term || decoded.Error.Role != fixture.role {
								t.Fatalf("native block=%#v error=%v", response, err)
							}
						} else if action.name == "cancellation" || strings.HasSuffix(action.name, "_no_match") {
							if !reflect.DeepEqual(response, unchanged) {
								t.Fatalf("native cancellation=%#v", response)
							}
						} else {
							want := functionalHTTPBody(t, fixture, action.want)
							if !reflect.DeepEqual(response, pluginapi.RequestInterceptResponse{Body: want, Headers: http.Header{"X-Preserved": {"SECRET header"}}}) {
								t.Fatalf("native rewrite=%#v", response)
							}
						}
					})
				})
			}
		}
	}
}
