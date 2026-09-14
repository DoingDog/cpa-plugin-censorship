package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/tidwall/gjson"
)

var okEnvelopeByteSink []byte

func TestSupportedPluginSchema(t *testing.T) {
	if got, want := pluginabi.SchemaVersion, uint32(5); got != want {
		t.Fatalf("plugin schema = %d, want %d", got, want)
	}
}

func TestOKEnvelopeExactBytesAndAllocationCeiling(t *testing.T) {
	type result struct {
		Value string `json:"value"`
	}

	for _, tc := range []struct {
		name  string
		value any
		want  []byte
	}{
		{name: "value", value: result{Value: "x"}, want: []byte(`{"ok":true,"result":{"value":"x"}}`)},
		{name: "null", value: nil, want: []byte(`{"ok":true,"result":null}`)},
		{
			name: "request intercept response",
			value: pluginapi.RequestInterceptResponse{
				Headers:         http.Header{"X-Test": {"<&", "z"}},
				Body:            []byte{0, 1, 2},
				ClearHeaders:    []string{"X-Old"},
				Terminate:       true,
				StatusCode:      400,
				ResponseHeaders: http.Header{"Content-Type": {"application/json"}},
				ResponseBody:    []byte(`{"error":"<bad>"}`),
			},
			want: []byte(`{"ok":true,"result":{"Headers":{"X-Test":["\u003c\u0026","z"]},"Body":"AAEC","ClearHeaders":["X-Old"],"Terminate":true,"StatusCode":400,"ResponseHeaders":{"Content-Type":["application/json"]},"ResponseBody":"eyJlcnJvciI6IjxiYWQ+In0="}}`),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := okEnvelope(tc.value)
			if err != nil {
				t.Fatalf("okEnvelope() error = %v", err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("okEnvelope() = %s, want %s", got, tc.want)
			}
		})
	}

	if raceDetectorEnabled {
		return
	}

	response := pluginapi.RequestInterceptResponse{Body: bytes.Repeat([]byte("x"), 1<<20)}
	allocs := testing.AllocsPerRun(100, func() {
		var err error
		okEnvelopeByteSink, err = okEnvelope(response)
		if err != nil {
			panic(err)
		}
	})
	if allocs > 4 {
		t.Fatalf("okEnvelope() allocations = %.1f, want <= 4", allocs)
	}
}

func TestRegistrationDeclaresOnlyRequestInterceptor(t *testing.T) {
	raw, err := handleMethod(pluginabi.MethodPluginRegister, []byte(`{"config_yaml":"","schema_version":4}`))
	if err != nil {
		t.Fatalf("handleMethod() error = %v", err)
	}
	var env pluginabi.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("register envelope = %s", raw)
	}
	var got struct {
		SchemaVersion uint32             `json:"schema_version"`
		Metadata      pluginapi.Metadata `json:"metadata"`
		Capabilities  map[string]bool    `json:"capabilities"`
	}
	if err := json.Unmarshal(env.Result, &got); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	if got.SchemaVersion != pluginabi.SchemaVersion || got.Metadata.Name != "censorship" || got.Metadata.Version != pluginVersion {
		t.Fatalf("registration = %#v", got)
	}
	if got.Metadata.Author == "" || got.Metadata.GitHubRepository == "" || got.Metadata.Logo != "https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png" || got.Metadata.ConfigFields == nil {
		t.Fatalf("metadata = %#v", got.Metadata)
	}
	if !got.Capabilities["request_interceptor"] {
		t.Fatalf("capabilities = %#v", got.Capabilities)
	}
	for _, name := range []string{"request_lifecycle_plugin", "response_interceptor", "response_stream_interceptor", "websocket_response_observer", "management_api", "model_router", "executor"} {
		if got.Capabilities[name] {
			t.Fatalf("capability %q unexpectedly enabled", name)
		}
	}
}

func TestRegistrationExposesEditableConfigFields(t *testing.T) {
	fields := pluginRegistration().Metadata.ConfigFields
	want := []struct {
		name        string
		typeName    pluginapi.ConfigFieldType
		description string
		enumValues  []string
	}{
		{name: "ignore_case", typeName: pluginapi.ConfigFieldTypeBoolean, description: "Match terms with Go unicode.SimpleFold equivalence (default false)."},
		{name: "words", typeName: pluginapi.ConfigFieldTypeObject, description: "Optional block, strip, and obfs arrays; an empty object has no rules."},
		{name: "scope", typeName: pluginapi.ConfigFieldTypeObject, description: "Optional object with formats and roles arrays; defaults to all supported formats and system, developer, and user roles."},
		{name: "obfs", typeName: pluginapi.ConfigFieldTypeObject, description: "Obfuscation object whose char is U+200B or U+2060; used by obfs rules (default U+200B)."},
		{name: "filter_mode", typeName: pluginapi.ConfigFieldTypeEnum, description: "Exclude matching requests from censorship, or include only matching requests (default exclude).", enumValues: []string{"exclude", "include"}},
		{name: "filter_logic", typeName: pluginapi.ConfigFieldTypeEnum, description: "Combine non-empty filter api-keys and models with or or and (default or); one non-empty list is used alone.", enumValues: []string{"or", "and"}},
		{name: "filter", typeName: pluginapi.ConfigFieldTypeObject, description: "Optional object with api-keys and models pattern arrays; * matches zero or more Unicode scalars and ? matches one. Empty arrays or an empty object disable request filtering. API-key patterns require authenticated caller_scope binding and are not masked by the standard panel."},
	}
	if len(fields) != len(want) {
		t.Fatalf("config field count = %d, want %d: %#v", len(fields), len(want), fields)
	}
	for i, field := range fields {
		if field.Name != want[i].name || field.Type != want[i].typeName || field.Description != want[i].description || !reflect.DeepEqual(field.EnumValues, want[i].enumValues) {
			t.Errorf("config field %d = %#v, want name=%q type=%q description=%q enum values=%#v", i, field, want[i].name, want[i].typeName, want[i].description, want[i].enumValues)
		}
		if field.Name == "mode" {
			t.Error("config fields must not expose global mode")
		}
		if field.Name == "words" && field.Type != pluginapi.ConfigFieldTypeObject {
			t.Errorf("words type = %q, want object", field.Type)
		}
	}
}

func TestRegistrationUsesBuildVersion(t *testing.T) {
	if got, want := pluginVersion, "0.0.0-dev"; got != want {
		t.Fatalf("source pluginVersion = %q, want %q", got, want)
	}

	original := pluginVersion
	t.Cleanup(func() { pluginVersion = original })
	pluginVersion = "v0.2.0"

	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginRegister, lifecycleJSON(t, "")), &env)
	got := decodeResult[registration](t, env)
	if got.Metadata.Version != pluginVersion {
		t.Fatalf("registration version = %q, want %q", got.Metadata.Version, pluginVersion)
	}
}

func mustHandle(t *testing.T, method string, request []byte) []byte {
	t.Helper()
	raw, err := handleMethod(method, request)
	if err != nil {
		t.Fatalf("handleMethod(%q): %v", method, err)
	}
	return raw
}

func decodeEnvelope(t *testing.T, raw []byte, env *pluginabi.Envelope) {
	t.Helper()
	if err := json.Unmarshal(raw, env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
}

func decodeResult[T any](t *testing.T, env pluginabi.Envelope) T {
	t.Helper()
	if !env.OK {
		t.Fatalf("RPC envelope error = %#v", env.Error)
	}
	var result T
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return result
}

func lifecycleJSON(t *testing.T, configYAML string) []byte {
	t.Helper()
	config := []byte(configYAML)
	raw, err := json.Marshal(lifecycleRequest{
		ConfigYAML:    &config,
		SchemaVersion: pluginabi.SchemaVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestBeforeAuthEarlyNoOpsDoNotParseBody(t *testing.T) {
	cases := []struct {
		name   string
		format string
		yaml   string
	}{
		{name: "unknown format", format: "future", yaml: "words: [x]\n"},
		{name: "empty words", format: "openai", yaml: "words: []\n"},
		{name: "empty formats", format: "openai", yaml: "words: [x]\nscope:\n  formats: []\n"},
		{name: "format disabled", format: "openai", yaml: "words: [x]\nscope:\n  formats: [claude]\n"},
		{name: "empty roles", format: "openai", yaml: "words: [x]\nscope:\n  roles: []\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, tc.yaml)
			resp := interceptRPC(t, tc.format, []byte(`not-json`))
			if resp.Terminate || len(resp.Body) != 0 || len(resp.ResponseBody) != 0 {
				t.Fatalf("response = %#v", resp)
			}
		})
	}
}

func TestBeforeAuthChangedBodyClearsStaleEntityHeaders(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	raw, err := json.Marshal(pluginapi.RequestInterceptRequest{
		RequestID:    "header-test",
		SourceFormat: "openai",
		Headers: http.Header{
			"Content-Encoding":  {"zstd"},
			"Content-Length":    {"123"},
			"Transfer-Encoding": {"chunked"},
			"Content-Type":      {"application/json"},
		},
		Body: []byte(`{"messages":[{"role":"user","content":"SECRET text"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodRequestInterceptBefore, raw), &env)
	resp := decodeResult[pluginapi.RequestInterceptResponse](t, env)
	wantClear := []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"}
	if !reflect.DeepEqual(resp.ClearHeaders, wantClear) {
		t.Fatalf("ClearHeaders = %#v, want %#v", resp.ClearHeaders, wantClear)
	}
	if got := string(resp.Body); got != `{"messages":[{"role":"user","content":" text"}]}` {
		t.Fatalf("body = %s", resp.Body)
	}

	noChange := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"clean"}]}`))
	if noChange.ClearHeaders != nil {
		t.Fatalf("no-op ClearHeaders = %#v, want nil", noChange.ClearHeaders)
	}
}

func TestBeforeAuthRejectsEnabledInvalidJSON(t *testing.T) {
	registerConfig(t, "words: [x]\n")
	for _, body := range [][]byte{[]byte(`not-json`), []byte(`[]`), []byte(`null`), []byte(`"x"`)} {
		resp := interceptRPC(t, "openai", body)
		if !resp.Terminate || resp.StatusCode != 400 || resp.ResponseHeaders.Get("Content-Type") != "application/json" {
			t.Fatalf("response for %q = %#v", body, resp)
		}
		if gjson.GetBytes(resp.ResponseBody, "error.code").String() != "censorship_invalid_request" || bytes.Contains(resp.ResponseBody, body) {
			t.Fatalf("response body = %s", resp.ResponseBody)
		}
	}
}

func TestBeforeAuthRejectsDuplicateJSONMembers(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	for _, body := range [][]byte{
		[]byte(`{"messages":[],"messages":[{"role":"user","content":"SECRET"}]}`),
		[]byte(`{"messages":[{"role":"assistant","role":"user","content":"SECRET"}]}`),
		[]byte(`{"messages":[{"role":"user","content":"clean","content":"SECRET"}]}`),
		[]byte(`{"messages":[{"role":"user","content":"SECRET","n":1e10000}],"messages":[]}`),
	} {
		resp := interceptRPC(t, "openai", body)
		if !resp.Terminate || resp.StatusCode != 400 || resp.ResponseHeaders.Get("Content-Type") != "application/json" {
			t.Fatalf("response for %q = %#v", body, resp)
		}
		if gjson.GetBytes(resp.ResponseBody, "error.code").String() != "censorship_invalid_request" || bytes.Contains(resp.ResponseBody, body) {
			t.Fatalf("response body = %s", resp.ResponseBody)
		}
	}
}

func TestDuplicateJSONMembersInsideStringRemainOpaque(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	body := []byte(`{"messages":[{"role":"user","content":"{\"x\":1,\"x\":2} SECRET"}]}`)
	resp := interceptRPC(t, "openai", body)
	if resp.Terminate || string(resp.Body) != `{"messages":[{"role":"user","content":"{\"x\":1,\"x\":2} "}]}` {
		t.Fatalf("response = %#v", resp)
	}
}

func TestDuplicateMemberWalkerAcceptsParsedRoot(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
		want bool
	}{
		{name: "duplicate member", body: []byte(`{"x":0,"x":1}`), want: true},
		{name: "sibling objects", body: []byte(`{"left":{"x":0},"right":{"x":1}}`)},
		{name: "array nesting", body: []byte(`{"items":[{"x":0},{"nested":{"x":1,"x":2}}]}`), want: true},
		{name: "JSON-looking string", body: []byte(`{"value":"{\"x\":1,\"x\":2}"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasDuplicateJSONMembers(gjson.ParseBytes(tc.body)); got != tc.want {
				t.Fatalf("hasDuplicateJSONMembers(%s) = %t, want %t", tc.body, got, tc.want)
			}
		})
	}
}

func TestDuplicateMemberWalkerCanonicalizesMemberNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
		want bool
	}{
		{name: "escaped ASCII", body: []byte(`{"x":0,"\u0078":1}`), want: true},
		{name: "lone surrogate followed by escape", body: []byte(`{"\ud800\u0061":0,"\ufffda":1}`), want: true},
		{name: "lone surrogate differs from replacement only", body: []byte(`{"\ud800\u0061":0,"\ufffd":1}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasDuplicateJSONMembers(gjson.ParseBytes(tc.body)); got != tc.want {
				t.Fatalf("hasDuplicateJSONMembers(%s) = %t, want %t", tc.body, got, tc.want)
			}
		})
	}
}

func TestDuplicateMemberWalkerChecksExcludedSubtrees(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{name: "tools", body: []byte(`{"messages":[],"tools":[{"x":0,"x":1}]}`)},
		{name: "media", body: []byte(`{"messages":[],"media":{"x":0,"x":1}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !hasDuplicateJSONMembers(gjson.ParseBytes(tc.body)) {
				t.Fatalf("hasDuplicateJSONMembers(%s) = false, want true", tc.body)
			}
		})
	}
}

func TestModelFilterDefersBeforeAuthBeforeEnvelopeDecode(t *testing.T) {
	registerConfig(t, "filter_mode: include\nfilter:\n  models: [target-model]\nwords:\n  block: [blocked]\n")

	raw, err := interceptBeforeAuth([]byte(`not-json`))
	if err != nil {
		t.Fatal(err)
	}
	var env pluginabi.Envelope
	decodeEnvelope(t, raw, &env)
	requireNoOpResponse(t, decodeResult[pluginapi.RequestInterceptResponse](t, env))

	response, err := callInterceptRequest(pluginapi.RequestInterceptRequest{
		RequestID:      "before-malformed-body",
		SourceFormat:   "openai",
		Model:          "target-model",
		RequestedModel: "requested-decoy",
		Body:           []byte(`not-json`),
	})
	if err != nil {
		t.Fatal(err)
	}
	requireNoOpResponse(t, response)
}

func TestAfterAuthFastPathsBeforeEnvelopeDecode(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configYAML string
	}{
		{name: "empty filter", configYAML: "words:\n  block: [blocked]\n"},
		{name: "API-key-only filter", configYAML: "filter_mode: include\nfilter:\n  api-keys: [test-key]\nwords:\n  block: [blocked]\n"},
		{name: "model filter with zero rules", configYAML: "filter_mode: include\nfilter:\n  models: [target-model]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, tc.configYAML)
			raw, err := interceptAfterAuth([]byte(`not-json`))
			if err != nil {
				t.Fatal(err)
			}
			var env pluginabi.Envelope
			decodeEnvelope(t, raw, &env)
			requireNoOpResponse(t, decodeResult[pluginapi.RequestInterceptResponse](t, env))
		})
	}
}

func TestModelFilterProcessesOnlySelectedAuthAfter(t *testing.T) {
	registerConfig(t, "filter_mode: include\nfilter:\n  models: [target-model]\nwords:\n  block: [blocked]\n")

	for _, tc := range []struct {
		name     string
		metadata map[string]any
		body     []byte
		active   bool
	}{
		{name: "missing marker", body: []byte(`not-json`)},
		{name: "null marker", metadata: map[string]any{executor.SelectedAuthMetadataKey: nil}, body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`)},
		{name: "empty marker", metadata: map[string]any{executor.SelectedAuthMetadataKey: ""}, body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`)},
		{name: "whitespace marker", metadata: map[string]any{executor.SelectedAuthMetadataKey: "   "}, body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`)},
		{name: "integer marker", metadata: map[string]any{executor.SelectedAuthMetadataKey: 1}, body: []byte(`not-json`)},
		{name: "null selected auth index", metadata: map[string]any{executor.SelectedAuthIndexMetadataKey: nil}, body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`)},
		{name: "empty selected auth index", metadata: map[string]any{executor.SelectedAuthIndexMetadataKey: ""}, body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`)},
		{name: "whitespace selected auth index", metadata: map[string]any{executor.SelectedAuthIndexMetadataKey: "   "}, body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`)},
		{name: "integer selected auth index", metadata: map[string]any{executor.SelectedAuthIndexMetadataKey: 1}, body: []byte(`not-json`)},
		{name: "selected auth ID", metadata: map[string]any{executor.SelectedAuthMetadataKey: "auth-1", "source": "source-decoy"}, body: []byte(`{"model":"body-decoy","messages":[{"role":"user","content":"blocked"}]}`), active: true},
		{name: "selected auth index", metadata: map[string]any{executor.SelectedAuthIndexMetadataKey: "index-1", "source": "source-decoy"}, body: []byte(`{"model":"body-decoy","messages":[{"role":"user","content":"blocked"}]}`), active: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := callInterceptRequestAt(pluginabi.MethodRequestInterceptAfter, pluginapi.RequestInterceptRequest{
				RequestID:      "selected-auth-" + tc.name,
				SourceFormat:   "openai",
				Model:          "target-model",
				RequestedModel: "requested-decoy",
				Metadata:       tc.metadata,
				Body:           tc.body,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !tc.active {
				requireNoOpResponse(t, response)
				return
			}
			if !response.Terminate || response.StatusCode != 400 || !strings.Contains(string(response.ResponseBody), "censorship_blocked") {
				t.Fatalf("response = %#v, want terminated censorship_blocked", response)
			}
		})
	}

	response, err := callInterceptRequestAt(pluginabi.MethodRequestInterceptAfter, pluginapi.RequestInterceptRequest{
		RequestID:      "selected-auth-requested-model-decoy",
		SourceFormat:   "openai",
		Model:          "model-decoy",
		RequestedModel: "target-model",
		Metadata:       selectedAuthMetadata(),
		Body:           []byte(`{"model":"target-model","messages":[{"role":"user","content":"blocked"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	requireNoOpResponse(t, response)

	response, err = callInterceptRequestAt(pluginabi.MethodRequestInterceptAfter, pluginapi.RequestInterceptRequest{
		RequestID:      "selected-auth-unknown-format",
		SourceFormat:   "unknown",
		Model:          "target-model",
		RequestedModel: "requested-decoy",
		Metadata:       selectedAuthMetadata(),
		Body:           []byte(`not-json`),
	})
	if err != nil {
		t.Fatal(err)
	}
	requireNoOpResponse(t, response)
}

func TestModelAndAPIKeyFiltersRunTogetherAfterAuth(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configYAML string
		request    pluginapi.RequestInterceptRequest
		active     bool
	}{
		{
			name:       "and exact caller scope and matching model",
			configYAML: "filter_mode: include\nfilter_logic: and\nfilter:\n  api-keys: [test-key]\n  models: [target-model]\nwords:\n  block: [blocked]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "and-filter",
				SourceFormat:   "openai",
				Model:          "target-model",
				RequestedModel: "requested-decoy",
				Metadata: map[string]any{
					executor.SelectedAuthMetadataKey: "auth-1",
					callerScopeMetadataKey:           testKeyCallerScope,
				},
				Body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`),
			},
			active: true,
		},
		{
			name:       "and exact caller scope mismatch and matching model",
			configYAML: "filter_mode: include\nfilter_logic: and\nfilter:\n  api-keys: [test-key]\n  models: [target-model]\nwords:\n  block: [blocked]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "and-filter-scope-decoy",
				SourceFormat:   "openai",
				Model:          "target-model",
				RequestedModel: "requested-decoy",
				Metadata: map[string]any{
					executor.SelectedAuthMetadataKey: "auth-1",
					callerScopeMetadataKey:           testKeyCallerScope + "-decoy",
				},
				Body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`),
			},
		},
		{
			name:       "or wildcard authorization and matching model",
			configYAML: "filter_mode: include\nfilter_logic: or\nfilter:\n  api-keys: [test-*]\n  models: [target-model]\nwords:\n  block: [blocked]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "or-filter",
				SourceFormat:   "openai",
				Model:          "target-model",
				RequestedModel: "requested-decoy",
				Headers:        http.Header{"Authorization": {"Bearer test-key"}},
				Metadata: map[string]any{
					executor.SelectedAuthMetadataKey: "auth-1",
					callerScopeMetadataKey:           testKeyCallerScope,
				},
				Body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`),
			},
			active: true,
		},
		{
			name:       "or wildcard authorization and requested model decoy",
			configYAML: "filter_mode: include\nfilter_logic: or\nfilter:\n  api-keys: [test-*]\n  models: [target-model]\nwords:\n  block: [blocked]\n",
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "or-filter-model-decoy",
				SourceFormat:   "openai",
				Model:          "model-decoy",
				RequestedModel: "target-model",
				Headers:        http.Header{"Authorization": {"Bearer test-key"}},
				Metadata: map[string]any{
					executor.SelectedAuthMetadataKey: "auth-1",
					callerScopeMetadataKey:           testKeyCallerScope,
				},
				Body: []byte(`{"messages":[{"role":"user","content":"blocked"}]}`),
			},
			active: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, tc.configYAML)
			response, err := callInterceptRequestAt(pluginabi.MethodRequestInterceptAfter, tc.request)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.active {
				requireNoOpResponse(t, response)
			} else if !response.Terminate || response.StatusCode != 400 || !strings.Contains(string(response.ResponseBody), "censorship_blocked") {
				t.Fatalf("AfterAuth response = %#v, want terminated censorship_blocked", response)
			}

			response, err = callInterceptRequestAt(pluginabi.MethodRequestInterceptBefore, tc.request)
			if err != nil {
				t.Fatal(err)
			}
			requireNoOpResponse(t, response)
		})
	}
}

func registerConfig(t *testing.T, configYAML string) {
	t.Helper()
	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginRegister, lifecycleJSON(t, configYAML)), &env)
	if !env.OK {
		t.Fatalf("register config: %#v", env.Error)
	}
}

func callIntercept(sourceFormat string, body []byte) (pluginapi.RequestInterceptResponse, error) {
	return callInterceptRequest(pluginapi.RequestInterceptRequest{
		RequestID:    "censorship-test",
		SourceFormat: sourceFormat,
		Body:         body,
	})
}

func callInterceptRequest(request pluginapi.RequestInterceptRequest) (pluginapi.RequestInterceptResponse, error) {
	return callInterceptRequestAt(pluginabi.MethodRequestInterceptBefore, request)
}

func callInterceptRequestAt(method string, request pluginapi.RequestInterceptRequest) (pluginapi.RequestInterceptResponse, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	envelopeBytes, err := handleMethod(method, raw)
	if err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	var env pluginabi.Envelope
	if err := json.Unmarshal(envelopeBytes, &env); err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	if !env.OK {
		return pluginapi.RequestInterceptResponse{}, fmt.Errorf("RPC %s: %s", env.Error.Code, env.Error.Message)
	}
	var resp pluginapi.RequestInterceptResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	return resp, nil
}

func requireNoOpResponse(t testing.TB, response pluginapi.RequestInterceptResponse) {
	t.Helper()
	if !reflect.DeepEqual(response, pluginapi.RequestInterceptResponse{}) {
		t.Fatalf("response = %#v, want zero-value response", response)
	}
}

func selectedAuthMetadata() map[string]any {
	return map[string]any{executor.SelectedAuthMetadataKey: "auth-1"}
}

func interceptRPC(t *testing.T, sourceFormat string, body []byte) pluginapi.RequestInterceptResponse {
	t.Helper()
	resp, err := callIntercept(sourceFormat, body)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

type rawReplacement struct {
	Before string
	After  string
}

func replaceRawTokens(t *testing.T, body []byte, replacements ...rawReplacement) []byte {
	t.Helper()
	out := bytes.Clone(body)
	for _, replacement := range replacements {
		before := []byte(replacement.Before)
		if count := bytes.Count(out, before); count != 1 {
			t.Fatalf("raw token %q occurs %d times", before, count)
		}
		out = bytes.Replace(out, before, []byte(replacement.After), 1)
	}
	return out
}

func TestBeforeAuthBlockIgnoreCaseReturnsYAMLTerm(t *testing.T) {
	registerConfig(t, "mode: block\nignore_case: true\nwords: [Alpha]\n")
	resp := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"aLPHA"}]}`))
	if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.term").String() != "Alpha" || gjson.GetBytes(resp.ResponseBody, "error.role").String() != "user" {
		t.Fatalf("response = %#v body = %s", resp, resp.ResponseBody)
	}
}

func TestReconfigureStoresValidSnapshotAndKeepsLastKnownGoodOnError(t *testing.T) {
	const rawCredential = "secret-sentinel"
	intercept := func(model, content string) pluginapi.RequestInterceptResponse {
		t.Helper()
		resp, err := callInterceptRequestAt(pluginabi.MethodRequestInterceptAfter, pluginapi.RequestInterceptRequest{
			RequestID:      "filter-lifecycle-test",
			SourceFormat:   "openai",
			Model:          model,
			RequestedModel: "requested-decoy",
			Metadata:       selectedAuthMetadata(),
			Body:           []byte(`{"messages":[{"role":"user","content":"` + content + `"}]}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	registerConfig(t, "filter_mode: include\nfilter: {models: [model-a]}\nwords: {block: [alpha]}\n")
	if got := intercept("model-a", "alpha"); !got.Terminate {
		t.Fatal("config A did not block model-a alpha")
	}
	if got := intercept("model-b", "alpha"); got.Terminate {
		t.Fatalf("config A did not bypass model-b: %#v", got)
	}

	invalidConfig := "filter: {api-keys: [" + rawCredential + ", '']}\n"
	invalidRegister := mustHandle(t, pluginabi.MethodPluginRegister, lifecycleJSON(t, invalidConfig))
	var registerEnv pluginabi.Envelope
	decodeEnvelope(t, invalidRegister, &registerEnv)
	if registerEnv.OK || registerEnv.Error == nil || registerEnv.Error.Code != "plugin_error" {
		t.Fatalf("invalid register envelope = %#v", registerEnv)
	}
	if bytes.Contains(invalidRegister, []byte(rawCredential)) {
		t.Fatalf("invalid register envelope contains credential: %s", invalidRegister)
	}
	if got := intercept("model-a", "alpha"); !got.Terminate {
		t.Fatalf("invalid register replaced config A: %#v", got)
	}

	reconfigureConfig(t, "filter_mode: exclude\nfilter: {models: [model-a]}\nignore_case: true\nwords: {block: [Beta]}\n")
	if got := intercept("model-a", "BETA"); got.Terminate {
		t.Fatalf("config B did not bypass model-a: %#v", got)
	}
	if got := intercept("model-b", "BETA"); !got.Terminate || !bytes.Contains(got.ResponseBody, []byte(`"term":"Beta"`)) {
		t.Fatalf("config B response = %#v", got)
	}

	var logs []hostLogRequest
	setHostCallbackForTest(func(method string, request []byte) ([]byte, error) {
		if method == pluginabi.MethodHostLog {
			var req hostLogRequest
			if err := json.Unmarshal(request, &req); err != nil {
				return nil, err
			}
			logs = append(logs, req)
		}
		return okEnvelope(struct{}{})
	})
	t.Cleanup(func() { setHostCallbackForTest(nil) })
	reconfigureConfig(t, invalidConfig)
	if got := intercept("model-b", "BETA"); !got.Terminate || !bytes.Contains(got.ResponseBody, []byte(`"term":"Beta"`)) {
		t.Fatalf("invalid reconfigure replaced config B: %#v", got)
	}
	if len(logs) != 1 || logs[0].Level != "error" || logs[0].Message != "censorship plugin reconfigure rejected" {
		t.Fatalf("host logs = %#v", logs)
	}
	logError := fmt.Sprint(logs[0].Fields["error"])
	if !strings.Contains(logError, "filter.api-keys") || strings.Contains(logError, rawCredential) || strings.Contains(logError, "caller_scope") {
		t.Fatalf("host log error = %q", logError)
	}
}

func requirePhaseClassReconfigureLog(t testing.TB, logs []hostLogRequest, yamlTerms ...string) {
	t.Helper()
	if len(logs) != 1 || logs[0].Level != "error" || logs[0].Message != "censorship plugin reconfigure rejected" {
		t.Fatalf("host logs = %#v", logs)
	}
	if got := fmt.Sprint(logs[0].Fields["error"]); got != "filter.models phase change requires restart" {
		t.Fatalf("host log error = %q", got)
	}
	fields := fmt.Sprint(logs[0].Fields)
	for _, term := range yamlTerms {
		if strings.Contains(fields, term) {
			t.Fatalf("host log fields contain YAML term %q: %s", term, fields)
		}
	}
}

func TestReconfigureRejectsModelFilterPhaseClassChanges(t *testing.T) {
	for _, tc := range []struct {
		name              string
		initialConfigYAML string
		reconfigureYAML   string
		method            string
		request           pluginapi.RequestInterceptRequest
	}{
		{
			name:              "non-empty to empty",
			initialConfigYAML: "filter_mode: include\nfilter:\n  models: [target-model]\nwords:\n  block: [old-block]\n",
			reconfigureYAML:   "words:\n  block: [new-block]\n",
			method:            pluginabi.MethodRequestInterceptAfter,
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "phase-nonempty-to-empty",
				SourceFormat:   "openai",
				Model:          "target-model",
				RequestedModel: "requested-decoy",
				Metadata:       selectedAuthMetadata(),
				Body:           []byte(`{"messages":[{"role":"user","content":"old-block"}]}`),
			},
		},
		{
			name:              "empty to non-empty",
			initialConfigYAML: "words:\n  block: [old-block]\n",
			reconfigureYAML:   "filter_mode: include\nfilter:\n  models: [target-model]\nwords:\n  block: [new-block]\n",
			method:            pluginabi.MethodRequestInterceptBefore,
			request: pluginapi.RequestInterceptRequest{
				RequestID:      "phase-empty-to-nonempty",
				SourceFormat:   "openai",
				Model:          "target-model",
				RequestedModel: "requested-decoy",
				Body:           []byte(`{"messages":[{"role":"user","content":"old-block"}]}`),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, tc.initialConfigYAML)
			var logs []hostLogRequest
			setHostCallbackForTest(func(method string, request []byte) ([]byte, error) {
				if method == pluginabi.MethodHostLog {
					var req hostLogRequest
					if err := json.Unmarshal(request, &req); err != nil {
						return nil, err
					}
					logs = append(logs, req)
				}
				return okEnvelope(struct{}{})
			})
			t.Cleanup(func() { setHostCallbackForTest(nil) })

			reconfigureConfig(t, tc.reconfigureYAML)
			requirePhaseClassReconfigureLog(t, logs, "target-model", "old-block", "new-block")

			response, err := callInterceptRequestAt(tc.method, tc.request)
			if err != nil {
				t.Fatal(err)
			}
			if !response.Terminate || response.StatusCode != 400 || gjson.GetBytes(response.ResponseBody, "error.term").String() != "old-block" {
				t.Fatalf("response = %#v, want old snapshot block", response)
			}
		})
	}
}

func TestReconfigureAllowsSameModelFilterPhaseClass(t *testing.T) {
	t.Run("empty to empty", func(t *testing.T) {
		registerConfig(t, "words:\n  block: [old-block]\n")
		reconfigureConfig(t, "words:\n  strip: [new-strip]\n")
		response, err := callInterceptRequest(pluginapi.RequestInterceptRequest{
			RequestID:    "same-empty",
			SourceFormat: "openai",
			Body:         []byte(`{"messages":[{"role":"user","content":"new-strip"}]}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if response.Terminate || string(response.Body) != `{"messages":[{"role":"user","content":""}]}` {
			t.Fatalf("response = %#v, want reconfigured strip response", response)
		}
	})

	t.Run("non-empty to non-empty", func(t *testing.T) {
		registerConfig(t, "filter_mode: include\nfilter:\n  models: [target-model]\nwords:\n  block: [old-block]\n")
		reconfigureConfig(t, "filter_mode: include\nfilter:\n  models: [target-model]\nwords:\n  strip: [new-strip]\n")
		response, err := callInterceptRequestAt(pluginabi.MethodRequestInterceptAfter, pluginapi.RequestInterceptRequest{
			RequestID:      "same-nonempty",
			SourceFormat:   "openai",
			Model:          "target-model",
			RequestedModel: "requested-decoy",
			Metadata:       selectedAuthMetadata(),
			Body:           []byte(`{"messages":[{"role":"user","content":"new-strip"}]}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if response.Terminate || string(response.Body) != `{"messages":[{"role":"user","content":""}]}` {
			t.Fatalf("response = %#v, want reconfigured strip response", response)
		}
	})
}

func TestRegisterYAMLDecodeErrorDoesNotDiscloseFilterSourceToken(t *testing.T) {
	const sourceToken = "secret-sentinel"
	raw := mustHandle(t, pluginabi.MethodPluginRegister, lifecycleJSON(t, "filter: {api-keys: [*"+sourceToken+"]}\n"))
	var env pluginabi.Envelope
	decodeEnvelope(t, raw, &env)
	if env.OK || env.Error == nil || env.Error.Code != "plugin_error" {
		t.Fatalf("register envelope = %#v", env)
	}
	if bytes.Contains(raw, []byte(sourceToken)) {
		t.Fatalf("register envelope contains source token: %s", raw)
	}
	if env.Error.Message != "config YAML decode failed" {
		t.Fatalf("register error = %q", env.Error.Message)
	}
}

func TestFilterMappingErrorsDoNotDiscloseSourceToken(t *testing.T) {
	const sourceToken = "secret-sentinel"
	t.Run("unknown filter key in register response", func(t *testing.T) {
		raw := mustHandle(t, pluginabi.MethodPluginRegister, lifecycleJSON(t, "filter: {"+sourceToken+": []}\n"))
		var env pluginabi.Envelope
		decodeEnvelope(t, raw, &env)
		if env.OK || env.Error == nil || env.Error.Code != "plugin_error" {
			t.Fatalf("register envelope = %#v", env)
		}
		if bytes.Contains(raw, []byte(sourceToken)) {
			t.Fatalf("register envelope contains source token: %s", raw)
		}
		if env.Error.Message != "filter: unknown key" {
			t.Fatalf("register error = %q", env.Error.Message)
		}
	})

	t.Run("duplicate filter key in reconfigure log retains snapshot", func(t *testing.T) {
		registerConfig(t, "mode: block\nwords: [retained]\n")
		var logs []hostLogRequest
		setHostCallbackForTest(func(method string, request []byte) ([]byte, error) {
			if method == pluginabi.MethodHostLog {
				var req hostLogRequest
				if err := json.Unmarshal(request, &req); err != nil {
					return nil, err
				}
				logs = append(logs, req)
			}
			return okEnvelope(struct{}{})
		})
		t.Cleanup(func() { setHostCallbackForTest(nil) })

		reconfigureConfig(t, "filter:\n  "+sourceToken+": []\n  "+sourceToken+": []\n")
		if len(logs) != 1 || logs[0].Level != "error" || logs[0].Message != "censorship plugin reconfigure rejected" {
			t.Fatalf("host logs = %#v", logs)
		}
		logError := fmt.Sprint(logs[0].Fields["error"])
		if strings.Contains(logError, sourceToken) {
			t.Fatalf("reconfigure log contains source token: %q", logError)
		}
		if logError != "filter: duplicate key" {
			t.Fatalf("reconfigure log error = %q", logError)
		}
		if got := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"retained"}]}`)); !got.Terminate || !bytes.Contains(got.ResponseBody, []byte(`"term":"retained"`)) {
			t.Fatalf("invalid reconfigure replaced last-known-good snapshot: %#v", got)
		}
	})
}

func TestReconfigureRejectsMissingConfigYAMLAndKeepsLastKnownGood(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{name: "null request", raw: `null`},
		{name: "empty object", raw: `{}`},
		{name: "missing config_yaml", raw: `{"schema_version":4}`},
		{name: "null config_yaml", raw: `{"config_yaml":null,"schema_version":4}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: block\nwords: [alpha-only]\n")
			var logs []hostLogRequest
			setHostCallbackForTest(func(method string, request []byte) ([]byte, error) {
				if method == pluginabi.MethodHostLog {
					var req hostLogRequest
					if err := json.Unmarshal(request, &req); err != nil {
						return nil, err
					}
					logs = append(logs, req)
				}
				return okEnvelope(struct{}{})
			})
			t.Cleanup(func() { setHostCallbackForTest(nil) })

			var env pluginabi.Envelope
			decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginReconfigure, []byte(tc.raw)), &env)
			_ = decodeResult[registration](t, env)
			if len(logs) != 1 || logs[0].Level != "error" || logs[0].Message != "censorship plugin reconfigure rejected" || logs[0].Fields["error"] == "" {
				t.Fatalf("host logs = %#v", logs)
			}
			got := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"alpha-only"}]}`))
			if !got.Terminate || !bytes.Contains(got.ResponseBody, []byte(`"term":"alpha-only"`)) {
				t.Fatalf("malformed reconfigure replaced last-known-good: %#v", got)
			}
		})
	}
}

func reconfigureConfig(t *testing.T, configYAML string) {
	t.Helper()
	var env pluginabi.Envelope
	decodeEnvelope(t, mustHandle(t, pluginabi.MethodPluginReconfigure, lifecycleJSON(t, configYAML)), &env)
	if !env.OK {
		t.Fatalf("reconfigure: %#v", env.Error)
	}
}

func TestDocumentationListsConfigAndLimits(t *testing.T) {
	const configExample = `plugins:
  enabled: true
  configs:
    censorship:
      enabled: true
      ignore_case: false
      words:
        block: [example]
        strip: []
        obfs: []
      scope:
        formats: [openai, openai-response, claude, gemini, interactions]
        roles: [system, developer, user]
      obfs:
        char: "​"
      filter_mode: exclude
      filter_logic: or
      filter: {}`

	required := []string{
		configExample,
		"Object-only rule editor",
		"does not render or emit global `mode`",
		"legacy handwritten YAML",
		"Object `words`, a supplied `mode` remains syntax-validated but is behaviorally ignored.",
		"block -> strip -> obfs",
		"OpenAI explicit text paths:",
		"Claude explicit text paths:",
		"Gemini explicit text paths:",
		"Interactions explicit text paths:",
		"Machine exclusions:",
		"CLIProxyAPI v7.2.152, schema 5",
		"c76dfd4e0edabab9000628b1560ab8ab379eadb8",
		"native ABI v1",
		"glibc 2.34+",
		"C.GoBytes",
		"does not claim pinned Windows host request-pointer liveness has been proven",
		"When censorship replaces a decoded request body, it clears `Content-Encoding`, `Content-Length`, and `Transfer-Encoding`; no-op requests preserve headers.",
		"The integration oracle strictly validates complete, correctly typed HTTP and Responses WebSocket JSON.",
		"optional repository `LICENSE` if one exists",
		"harness verifies upstream arrival, HTTP/1.1 EOF, chunk/trailer handling",
		"https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png",
		"no custom panel, menu, or Management API",
		"request-only; does not inspect live model output, response bodies, SSE output, or server WebSocket output.",
		"Explicit historical output fields replayed as later request input may be inspected under assistant scope.",
		"`enabled`: boolean; host-owned",
		"`mode`: string; default `block`",
		"`ignore_case`: boolean; default `false`",
		"With `ignore_case: false`, matching is a case-sensitive literal substring operation.",
		"`words`: Object with optional `block`, `strip`, and `obfs` arrays",
		"words is the only term source; the plugin has no built-in terms",
		"`scope.formats`: sequence of strings; default all five formats",
		"`scope.roles`: sequence of strings; default `system`, `developer`, and `user`",
		"CPA management clients expose `ignore_case`, `words`, `scope`, `obfs`, `filter_mode`, `filter_logic`, and `filter` through standard `ConfigFields`.",
		"`filter_mode`: string; default `exclude`. Allowed values are `exclude` and `include`.",
		"`filter_logic`: string; default `or`. Allowed values are `or` and `and`.",
		"`filter`: Object with optional `api-keys` and `models` arrays.",
		"An empty `filter: {}` or a filter with both lists empty disables request filtering and processes all requests.",
		"`api-keys` and `models` each apply list-internal OR.",
		"Across non-empty dimensions, `filter_logic: or` matches either dimension and `filter_logic: and` requires both dimensions.",
		"Full-string, case-sensitive glob matching uses `*` for zero or more Unicode scalars and `?` for exactly one Unicode scalar.",
		"| yes | yes | match | match |",
		"| yes | no | match | no match |",
		"| no | yes | match | no match |",
		"| no | no | no match | no match |",
		"| `exclude` | bypass | process |",
		"| `include` | process | bypass |",
		"`api-keys` matches authenticated `caller_scope`.",
		"`Principal` and literal credentials, including credentials carried in headers, do not match `caller_scope` when they differ.",
		"Query-only conditions support exact values only; wildcard query conditions are not supported.",
		"Management configuration readback returns filter values unchanged and does not mask secrets.",
		"A filter bypass returns no replacement body or header changes before JSON validation.",
		"OpenAI Responses `output_text` and `refusal` leaves use canonical `assistant` scope regardless of the source item role.",
		"Missing Gemini roles follow CPA's user/model alternation.",
		"Invalid Gemini roles advance CPA's user/model alternation but remain unselected.",
		"Interactions accepts camel-case `systemInstruction` when snake-case `system_instruction` is absent.",
		"Gemini machine exclusions include camelCase and snake_case non-null function, media, file, and code carriers; signature values remain excluded without excluding same-Part visible text.",
		"Enabled known formats reject JSON objects with duplicate member names at any nesting depth.",
		"`obfs.char`: must be `U+200B` or `U+2060`; default `U+200B`",
		"block checks rules before document order and returns the YAML term with canonical role",
		"strip and obfs process all leftmost non-overlapping occurrences before the next rule",
		"A final text or byte change to annotated Interactions `model_output.content` with non-empty annotations returns local `censorship_invalid_request`; exact cancellation back to the original bytes is allowed.",
		"obfs preserves original case and inserts after the first Unicode scalar",
		"unicode.SimpleFold",
		"Alpha/aLPHA",
		"Greek sigma",
		"Kelvin sign",
		"straße/STRASSE does not match",
		"no Unicode normalization",
		"assistant is inspected only when explicitly listed in scope.roles",
		"same-instance config-only reconfigure may update only within one `filter.models` phase class",
		"invalid reconfiguration keeps the last-known-good snapshot",
		"Machine exclusions: tool calls, machine schema values, arguments, reasoning, thinking, JSON keys, machine JSON, binary uploads, and image/audio/video/file base64 are never changed.",
		"1. hook is not raw ingress; document order follows current execution-body spans",
		"2. preprocessing can observe uncensored input",
		"3. Responses WebSocket covers only model-executed turns",
		"4. `generate=false` prewarm bypasses the plugin",
		"5. `/v1/realtime`, Live, sideband, and DataChannel bypass the plugin",
		"6. Alpha Search bypasses the plugin",
		"7. WebSocket block events omit `term` and `role`",
		"8. RequestInterceptor failures are fail-open",
		"10. Home mode does not watch local YAML",
		"11. unknown SourceFormat and future content types are not inspected; review schema drift when upgrading CPA",
		"censorship_<version>_<goos>_<goarch>.zip",
		".zip.sha256",
		"64 lowercase hex characters, two spaces, and the archive basename",
		"`program_output.result`",
		"`mcp_call.error.message`",
		"`mcp_list_tools.error`",
		"`file_search_call.results[*].text`",
		"`code_interpreter_call.outputs[type=logs].logs`",
		"`search_result.title`",
		"`document.title`",
		"`document.context`",
		"scalar `document.source.content`",
		"censorship rewrite would make a text field invalid",
		"0.0.0-dev",
		"aggregate packaging",
		"An aggregate packaging run covers exactly its present source platforms; a complete GitHub release/workflow contains all seven supported platform lines and assets.",
		"signed-history prefix",
		"The signature field and value remain excluded, but a non-null `thoughtSignature`",
		"Scalar Claude user content cannot be stripped to empty",
		"Responses prompt variables and local shell skill descriptions use canonical `user`",
		"Machine arguments, grammar definitions, names, IDs, paths, schema values, and reasoning state remain excluded.",
		"An unknown external hard-link peer of an existing destination is not modified",
	}
	forbidden := []string{
		"request-only; does not inspect model output",
		"length-changing rewrite to annotated Interactions",
		"## Seven-platform aggregate",
		"aggregate packaging run covers all seven supported platform",
		"and every model response",
	}
	readmeRequired := []string{
		"filter.models is empty",
		"filter.models is non-empty",
		"RequestAfterAuthInterceptor",
		"selected_auth_id",
		"selected_auth_index",
		"request.Model",
		"does not parse request-body model fields or implement CPA alias routing.",
		"non-stream host.model.execute",
		"HTTP 500",
		"filter.models phase change requires restart",
		"in-flight requests",
		"C.GoBytes",
		"every AfterAuth call that reads an input body performs synchronous `C.GoBytes` and incurs input-sized copy",
		"documented wildcard credential carriers",
		"Wildcard credential carriers are scanned in this exact order: `Authorization`, `X-Goog-Api-Key`, then `X-Api-Key`.",
		"`Authorization` accepts case-insensitive `Bearer <credential>` or a raw credential.",
		"Every wildcard candidate is trimmed and scope-bound before glob matching.",
		"Other headers and query-only credentials are unsupported wildcard carriers.",
	}
	for _, name := range []string{"README.md", "RELEASE_NOTES.md"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
		for _, token := range required {
			if !bytes.Contains(raw, []byte(token)) {
				t.Errorf("%s missing %q", name, token)
			}
		}
		for _, token := range forbidden {
			if bytes.Contains(raw, []byte(token)) {
				t.Errorf("%s contains superseded %q", name, token)
			}
		}
		if name == "README.md" && !bytes.Contains(raw, []byte("tool is inspected only for documented OpenAI, Claude, and Interactions result-text paths")) {
			t.Errorf("%s missing current tool result-text scope", name)
		}
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range readmeRequired {
		if !bytes.Contains(readme, []byte(token)) {
			t.Errorf("README.md missing %q", token)
		}
	}
	for _, token := range []string{
		"It examines selected text leaves in model request bodies before authentication.",
		"`RequestedModel` for ordinary model checks",
		"`Model` only when `Metadata[\"source\"]` equals `plugin_host_model_callback`",
		"valid non-Home YAML changes apply without restart",
		"After-auth intentionally does not read input.",
		"the plugin does not perform a C-to-Go input copy.",
	} {
		if bytes.Contains(readme, []byte(token)) {
			t.Errorf("README.md contains superseded current-contract claim %q", token)
		}
	}

	workflow, err := os.ReadFile(".github/workflows/build.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow = bytes.ReplaceAll(workflow, []byte("\r\n"), []byte("\n"))
	buildStart := bytes.Index(workflow, []byte("\n  build:\n"))
	buildEnd := bytes.Index(workflow, []byte("\n  build-windows-arm64:\n"))
	if buildStart < 0 || buildEnd < 0 || buildEnd <= buildStart {
		t.Fatal("build workflow job boundaries not found")
	}
	buildJob := workflow[buildStart:buildEnd]
	const abiSmokeStep = `      - name: Verify Windows amd64 active-after ABI
        if: "${{ matrix.GOOS == 'windows' && matrix.GOARCH == 'amd64' }}"
        shell: msys2 {0}
        run: go run ./.github/scripts/integration-runner.go -abi-smoke dist/windows_amd64/censorship.dll`
	if count := bytes.Count(buildJob, []byte(abiSmokeStep)); count != 1 {
		t.Fatalf("Windows amd64 ABI smoke step count = %d, want 1", count)
	}
	buildWindows := bytes.Index(buildJob, []byte("- name: Build and package on Windows"))
	abiSmoke := bytes.Index(buildJob, []byte(abiSmokeStep))
	upload := bytes.Index(buildJob, []byte("actions/upload-artifact@v4"))
	if buildWindows < 0 || abiSmoke < 0 || upload < 0 || buildWindows >= abiSmoke || abiSmoke >= upload {
		t.Fatal("Windows amd64 ABI smoke must run after package build and before artifact upload")
	}
}

func TestReleaseNotesCompatibilityTargetsCurrentAndHistoricalVersions(t *testing.T) {
	raw, err := os.ReadFile("RELEASE_NOTES.md")
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	const currentHeader = "# Censorship v0.3.2\n\n## v0.3.2 fixes\n"
	if !bytes.HasPrefix(raw, []byte(currentHeader)) {
		t.Fatal("RELEASE_NOTES.md does not begin with the v0.3.2 current-release section")
	}
	currentEnd := bytes.Index(raw, []byte("\n# Censorship v0.3.1\n"))
	if currentEnd < 0 {
		t.Fatal("RELEASE_NOTES.md does not separate the v0.3.2 current-release section")
	}
	currentRelease := raw[:currentEnd]
	for _, claim := range []string{
		"censorship only runs before authentication",
		"Outer request-interceptor calls match `RequestedModel`.",
		"only that nested call matches `Model`.",
		"valid non-Home YAML changes apply without restart",
		"After-auth intentionally does not read input.",
		"the plugin does not perform a C-to-Go input copy.",
	} {
		if bytes.Contains(currentRelease, []byte(claim)) {
			t.Errorf("RELEASE_NOTES.md current release contains superseded claim %q", claim)
		}
	}
	for _, sentence := range []string{
		"v0.3.2 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
		"v0.3.1 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
		"v0.3.0 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.",
	} {
		if !bytes.Contains(raw, []byte(sentence)) {
			t.Fatalf("RELEASE_NOTES.md does not contain compatibility sentence %q", sentence)
		}
	}

	const superseded = "> Superseded by `docs/superpowers/specs/2026-09-15-model-filter-post-route-execution-design.md`. Do not execute the v0.3.1 outer-`RequestedModel` plan."
	for _, name := range []string{
		"docs/superpowers/specs/2026-09-14-model-filter-plugin-compatibility-design.md",
		"docs/superpowers/plans/2026-09-14-model-filter-plugin-compatibility.md",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		raw = bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
		titleEnd := bytes.Index(raw, []byte("\n\n"))
		if titleEnd < 0 || !bytes.HasPrefix(raw[titleEnd:], []byte("\n\n"+superseded+"\n")) {
			t.Errorf("%s does not begin with the superseded notice", name)
		}
	}
}
