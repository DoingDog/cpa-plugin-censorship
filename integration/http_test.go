//go:build integration

package censorshipintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
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
