package main

import (
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
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
	raw, err := json.Marshal(lifecycleRequest{
		ConfigYAML:    []byte(configYAML),
		SchemaVersion: pluginabi.SchemaVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
