//go:build integration

package censorshipintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
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
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [alpha-only]\n")
	status, _, body := postChat(t, cpa, "alpha-only")
	if status != 400 || gjson.GetBytes(body, "error.term").String() != "alpha-only" {
		t.Fatalf("snapshot A did not block alpha-only: status=%d body=%s", status, body)
	}
	if upstream.arrivalCount() != 0 {
		t.Fatal("snapshot A block reached upstream")
	}

	watcherDeadline := time.Now().Add(20 * time.Second)
	for {
		writePluginConfig(t, cpa.config, "mode: block\nwords: [alpha-only, watcher-ready-only]\n")
		status, _, body = postChat(t, cpa, "watcher-ready-only")
		if status == 400 && gjson.GetBytes(body, "error.term").String() == "watcher-ready-only" {
			break
		}
		if time.Now().After(watcherDeadline) {
			t.Fatalf("config watcher not observed, last status=%d body=%s\n%s", status, body, readCPALog(cpa.logPath))
		}
		time.Sleep(time.Second)
	}

	writePluginConfig(t, cpa.config, "mode: block\nignore_case: true\nwords: [beta-only]\n")
	deadline := time.Now().Add(20 * time.Second)
	for {
		status, _, body = postChat(t, cpa, "BETA-ONLY")
		if status == 400 && gjson.GetBytes(body, "error.term").String() == "beta-only" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot B not observed, last status=%d body=%s\n%s", status, body, readCPALog(cpa.logPath))
		}
		time.Sleep(25 * time.Millisecond)
	}
	requestsBeforeAlpha := upstream.requestCount()
	status, _, body = postChat(t, cpa, "alpha-only")
	if status != 200 {
		t.Fatalf("snapshot B still blocked alpha-only: status=%d body=%s", status, body)
	}
	if got, want := upstream.requestCount(), requestsBeforeAlpha+1; got != want {
		t.Fatalf("snapshot B alpha-only request count = %d, want %d", got, want)
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
	if got == input {
		return fmt.Errorf("upstream message content remained input %q; body = %s", got, captured)
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
