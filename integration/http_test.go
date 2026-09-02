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
	writePluginConfig(t, cpa.config, "mode: block\nignore_case: true\nwords: [beta-only]\n")
	deadline := time.Now().Add(20 * time.Second)
	for {
		status, _, body := postChat(t, cpa, "BETA-ONLY")
		if status == 400 && gjson.GetBytes(body, "error.term").String() == "beta-only" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot B not observed, last status=%d body=%s\n%s", status, body, readCPALog(cpa.logPath))
		}
		time.Sleep(25 * time.Millisecond)
	}
	for i := 0; i < 50; i++ {
		status, _, body := postChat(t, cpa, "BETA-ONLY")
		if status != 400 || gjson.GetBytes(body, "error.term").String() != "beta-only" {
			t.Fatalf("post-linearization request %d saw non-B config: %d %s", i, status, body)
		}
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
