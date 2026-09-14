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

func TestAfterAuthAlwaysNoOpsWithoutParsingRequest(t *testing.T) {
	raw := mustHandle(t, pluginabi.MethodRequestInterceptAfter, []byte(`not-json`))
	var env pluginabi.Envelope
	decodeEnvelope(t, raw, &env)
	resp := decodeResult[pluginapi.RequestInterceptResponse](t, env)
	if resp.Terminate || len(resp.Body) != 0 || len(resp.Headers) != 0 || len(resp.ResponseBody) != 0 {
		t.Fatalf("AfterAuth response = %#v", resp)
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
	raw, err := json.Marshal(request)
	if err != nil {
		return pluginapi.RequestInterceptResponse{}, err
	}
	envelopeBytes, err := handleMethod(pluginabi.MethodRequestInterceptBefore, raw)
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
		resp, err := callInterceptRequest(pluginapi.RequestInterceptRequest{
			RequestID:      "filter-lifecycle-test",
			SourceFormat:   "openai",
			RequestedModel: model,
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
        char: "​"`

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
		"valid non-Home YAML changes apply without restart after observing a snapshot-B sentinel",
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
		"9. BeforeAuth runs once per handler execution; AfterAuth can run zero, one, or multiple times; every call carries the full body and incurs full-body RPC encoding/copy cost",
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
}

func TestReleaseNotesCompatibilityTargetsCurrentVersion(t *testing.T) {
	raw, err := os.ReadFile("RELEASE_NOTES.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("v0.2.6 targets CLIProxyAPI v7.2.152, schema 5, at host commit `c76dfd4e0edabab9000628b1560ab8ab379eadb8`. It uses native ABI v1. Linux artifacts require glibc 2.34+.")) {
		t.Fatal("RELEASE_NOTES.md does not contain the complete v0.2.6 compatibility sentence")
	}
}
