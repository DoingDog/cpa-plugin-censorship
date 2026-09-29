//go:build integration

package censorshipintegration

import (
	"testing"
)

func TestHTTPClaudeDescriptionsBlockBeforeUpstream(t *testing.T) {
	const config = "words:\n  block: [SECRET]\nscope:\n  roles: [system]\n"
	tests := []struct {
		name string
		body string
	}{
		{
			name: "tool description",
			body: `{"model":"censorship-integration-model","max_tokens":16,"messages":[{"role":"user","content":"safe"}],"tools":[{"name":"lookup","description":"SECRET lookup","input_schema":{"type":"object"}}]}`,
		},
		{
			name: "tool input property description",
			body: `{"model":"censorship-integration-model","max_tokens":16,"messages":[{"role":"user","content":"safe"}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"query":{"type":"string","description":"SECRET query"}}}}]}`,
		},
		{
			name: "output schema property description",
			body: `{"model":"censorship-integration-model","max_tokens":16,"messages":[{"role":"user","content":"safe"}],"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET answer"}}}}}}`,
		},
		{
			name: "legacy output_format schema property description",
			body: `{"model":"censorship-integration-model","max_tokens":16,"messages":[{"role":"user","content":"safe"}],"output_format":{"type":"json_schema","schema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET answer"}}}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := newMockUpstream(t)
			cpa := startCPA(t, upstream.URL, true, config)
			status, _, body := postJSON(t, cpa.baseURL+"/v1/messages", []byte(tt.body))
			if status != 400 {
				t.Errorf("status = %d, want 400; body = %s", status, body)
			}
			response, err := decodeCensorshipError(body)
			if err != nil {
				t.Errorf("decode censorship error: %v; body = %s", err, body)
			} else {
				if response.Error.Code != "censorship_blocked" {
					t.Errorf("error code = %q, want censorship_blocked", response.Error.Code)
				}
				if response.Error.Role != "system" {
					t.Errorf("error role = %q, want system", response.Error.Role)
				}
			}
			if arrivals := upstream.arrivalCount(); arrivals != 0 {
				t.Errorf("upstream arrivals = %d, want 0", arrivals)
			}
		})
	}
}
