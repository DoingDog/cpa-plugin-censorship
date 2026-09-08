//go:build integration

package censorshipintegration

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestHTTPBlockIncludesTermAndRole(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nwords: [Alpha]\nignore_case: true\n")
	status, header, body := postJSON(t, cpa.baseURL+"/v1/chat/completions", []byte(`{"model":"censorship-integration-model","messages":[{"role":"user","content":"aLPHA"}]}`))
	if status != 400 || header.Get("Content-Type") != "application/json" || gjson.GetBytes(body, "error.term").String() != "Alpha" || gjson.GetBytes(body, "error.role").String() != "user" {
		t.Fatalf("status=%d header=%v body=%s", status, header, body)
	}
	if upstream.requestCount() != 0 {
		t.Fatal("blocked request reached upstream")
	}
}

func TestHTTPRejectsDuplicateJSONMembers(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: strip\nwords: [SECRET]\n")
	body := []byte(`{"model":"censorship-integration-model","messages":[],"messages":[{"role":"user","content":"SECRET"}]}`)
	status, header, response := postJSON(t, cpa.baseURL+"/v1/chat/completions", body)
	if status != 400 || header.Get("Content-Type") != "application/json" || gjson.GetBytes(response, "error.code").String() != "censorship_invalid_request" {
		t.Fatalf("status=%d header=%v body=%s", status, header, response)
	}
	if upstream.requestCount() != 0 {
		t.Fatal("duplicate-member request reached upstream")
	}
}

func TestLegacyCompletionsPromptUsesConvertedUserRole(t *testing.T) {
	upstream := newMockUpstream(t)
	cpa := startCPA(t, upstream.URL, true, "mode: block\nignore_case: true\nwords: [Alpha]\n")
	status, _, body := postJSON(t, cpa.baseURL+"/v1/completions", []byte(`{"model":"censorship-integration-model","prompt":"aLPHA legacy prompt"}`))
	if status != 400 || gjson.GetBytes(body, "error.term").String() != "Alpha" || gjson.GetBytes(body, "error.role").String() != "user" {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if upstream.requestCount() != 0 {
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
	if upstream.requestCount() != 0 {
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
	if upstream.requestCount() != 0 {
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
			body := chatBody(tc.input, true)
			disabledTrace := captureHTTP11Trace(t, disabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			enabledTrace := captureHTTP11Trace(t, enabled.baseURL+"/v1/chat/completions", downstreamKey, body)
			if !bytes.Contains(enabledUpstream.lastRequest(), []byte(tc.transformed)) {
				t.Fatalf("upstream body = %s", enabledUpstream.lastRequest())
			}
			if !reflect.DeepEqual(enabledTrace, disabledTrace) {
				t.Fatalf("enabled trace = %#v, disabled trace = %#v", enabledTrace, disabledTrace)
			}
		})
	}
}
