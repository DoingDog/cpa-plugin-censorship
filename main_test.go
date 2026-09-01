package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/tidwall/gjson"
)

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
	if got.Metadata.Author == "" || got.Metadata.GitHubRepository == "" || got.Metadata.ConfigFields == nil || len(got.Metadata.ConfigFields) != 0 {
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
	raw, err := json.Marshal(pluginapi.RequestInterceptRequest{
		RequestID:    "censorship-test",
		SourceFormat: sourceFormat,
		Body:         body,
	})
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
	registerConfig(t, "mode: block\nwords: [alpha]\n")
	if got := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"alpha"}]}`)); !got.Terminate {
		t.Fatal("config A did not block alpha")
	}

	reconfigureConfig(t, "mode: block\nignore_case: true\nwords: [Beta]\n")
	if got := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"bETA"}]}`)); !got.Terminate || !bytes.Contains(got.ResponseBody, []byte(`"term":"Beta"`)) {
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
	reconfigureConfig(t, "ignore_case: yes\nwords: [gamma]\n")
	if got := interceptRPC(t, "openai", []byte(`{"messages":[{"role":"user","content":"BETA"}]}`)); !got.Terminate || !bytes.Contains(got.ResponseBody, []byte(`"term":"Beta"`)) {
		t.Fatalf("invalid B replaced last-known-good: %#v", got)
	}
	if len(logs) != 1 || logs[0].Level != "error" || logs[0].Message != "censorship plugin reconfigure rejected" || logs[0].Fields["error"] == "" {
		t.Fatalf("host logs = %#v", logs)
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
