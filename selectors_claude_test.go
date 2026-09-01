package main

import (
	"bytes"
	"testing"
)

func TestClaudeSelectorRowsAndExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, user, assistant, tool]\n")
	body := []byte(`{"system":[{"type":"text","text":"SECRET sys"},{"type":"thinking","thinking":"SECRET","text":"SECRET excluded"}],"messages":[
		{"role":"system","content":"SECRET message system"},
		{"role":"user","content":[
			{"type":"text","text":"SECRET user"},
			{"type":"tool_result","tool_use_id":"SECRET","content":[{"type":"text","text":"SECRET tool"},{"type":"image","source":{"data":"SECRET"}},{"type":"unknown","text":"SECRET unknown"}]},
			{"type":"tool_result","content":"SECRET string result"},
			{"type":"tool_result","content":{"text":"SECRET object result"}},
			{"type":"tool_use","name":"SECRET","input":{"text":"SECRET"}},
			{"type":"thinking","thinking":"SECRET"},
			{"type":"redacted_thinking","data":"SECRET"},
			{"type":"image","source":{"data":"SECRET"}}
		]},
		{"role":"assistant","content":"SECRET assistant"},
		{"role":"developer","content":"SECRET unsupported role"},
		{"role":"unknown","content":"SECRET unknown role"}
	]}`)
	resp := interceptRPC(t, "claude", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET sys"`, After: `" sys"`},
		rawReplacement{Before: `"SECRET message system"`, After: `" message system"`},
		rawReplacement{Before: `"SECRET user"`, After: `" user"`},
		rawReplacement{Before: `"SECRET tool"`, After: `" tool"`},
		rawReplacement{Before: `"SECRET assistant"`, After: `" assistant"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside contracted Claude string tokens")
	}
}

func TestClaudeTopLevelSystemStringAndToolScope(t *testing.T) {
	cases := []struct {
		name, roles, want string
	}{
		{name: "tool enabled", roles: "[system, tool]", want: `{"system":" sys","messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":" tool"}]}]}]}`},
		{name: "tool omitted", roles: "[system]", want: `{"system":" sys","messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"SECRET tool"}]}]}]}`},
	}
	body := []byte(`{"system":"SECRET sys","messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"SECRET tool"}]}]}]}`)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: "+tc.roles+"\n")
			resp := interceptRPC(t, "claude", body)
			if string(resp.Body) != tc.want {
				t.Fatalf("body = %s, want %s", resp.Body, tc.want)
			}
		})
	}
}

func TestClaudeSelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "system string", body: `{"system":"SECRET"}`, role: "system"},
		{name: "system block", body: `{"system":[{"type":"text","text":"SECRET"}]}`, role: "system"},
		{name: "message string", body: `{"messages":[{"role":"assistant","content":"SECRET"}]}`, role: "assistant"},
		{name: "message block", body: `{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET"}]}]}`, role: "user"},
		{name: "typed tool result", body: `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"SECRET"}]}]}]}`, role: "tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "claude", tc.body, tc.role) })
	}
}
