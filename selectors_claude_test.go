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
			{"type":"image","source":{"data":"SECRET"}},
			{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"SECRET"},"title":"SECRET document"}
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
		rawReplacement{Before: `"SECRET string result"`, After: `" string result"`},
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

func TestClaudeResultText(t *testing.T) {
	cases := []struct {
		name, roles, disabledRoles, body string
		replacements                     []rawReplacement
	}{
		{
			name:          "string tool result",
			roles:         "[tool]",
			disabledRoles: "[user]",
			body:          `{"messages":[{"role":"user","content":[{"type":"tool_result","content":"SECRET tool result"}]}]}`,
			replacements:  []rawReplacement{{Before: `"SECRET tool result"`, After: `" tool result"`}},
		},
		{
			name:          "direct user search result",
			roles:         "[user]",
			disabledRoles: "[tool]",
			body:          `{"messages":[{"role":"user","content":[{"type":"search_result","content":[{"type":"text","text":"SECRET direct search"}]}]}]}`,
			replacements:  []rawReplacement{{Before: `"SECRET direct search"`, After: `" direct search"`}},
		},
		{
			name:          "nested tool search result",
			roles:         "[tool]",
			disabledRoles: "[user]",
			body:          `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"search_result","content":[{"type":"text","text":"SECRET nested search"}]}]}]}]}`,
			replacements:  []rawReplacement{{Before: `"SECRET nested search"`, After: `" nested search"`}},
		},
		{
			name:          "direct user text document",
			roles:         "[user]",
			disabledRoles: "[tool]",
			body:          `{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"text","data":"SECRET direct document"}}]}]}`,
			replacements:  []rawReplacement{{Before: `"SECRET direct document"`, After: `" direct document"`}},
		},
		{
			name:          "nested tool text document",
			roles:         "[tool]",
			disabledRoles: "[user]",
			body:          `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"document","source":{"type":"text","data":"SECRET nested document"}}]}]}]}`,
			replacements:  []rawReplacement{{Before: `"SECRET nested document"`, After: `" nested document"`}},
		},
		{
			name:          "direct user custom-content document",
			roles:         "[user]",
			disabledRoles: "[tool]",
			body:          `{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"content","content":[{"type":"text","text":"SECRET direct first"},{"type":"image","source":{"data":"SECRET direct image"}},{"type":"text","text":"SECRET direct second"}]}}]}]}`,
			replacements: []rawReplacement{
				{Before: `"SECRET direct first"`, After: `" direct first"`},
				{Before: `"SECRET direct second"`, After: `" direct second"`},
			},
		},
		{
			name:          "nested tool custom-content document",
			roles:         "[tool]",
			disabledRoles: "[user]",
			body:          `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"document","source":{"type":"content","content":[{"type":"text","text":"SECRET nested first"},{"type":"image","source":{"data":"SECRET nested image"}},{"type":"text","text":"SECRET nested second"}]}}]}]}]}`,
			replacements: []rawReplacement{
				{Before: `"SECRET nested first"`, After: `" nested first"`},
				{Before: `"SECRET nested second"`, After: `" nested second"`},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: "+tc.roles+"\n")
			resp := interceptRPC(t, "claude", body)
			want := replaceRawTokens(t, body, tc.replacements...)
			if !bytes.Equal(resp.Body, want) {
				t.Fatalf("body = %s, want %s", resp.Body, want)
			}

			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: "+tc.disabledRoles+"\n")
			resp = interceptRPC(t, "claude", body)
			if resp.Body != nil || resp.Terminate {
				t.Fatalf("disabled role response = %#v", resp)
			}
		})
	}
}

func TestClaudeResultTextPreservesMachineFields(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user, tool]\n")
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"search_result","source":"SECRET search source","title":"SECRET search title","citations":[{"data":"SECRET citation data"}],"content":[{"type":"text","text":"SECRET direct search"},{"type":"image","source":{"data":"SECRET search non-text"}}]},{"type":"document","title":"SECRET document title","source":{"type":"text","media_type":"SECRET document media type","data":"SECRET direct document"}},{"type":"document","source":{"type":"url","url":"SECRET URL source"}},{"type":"document","source":{"type":"file","file_id":"SECRET file source"}},{"type":"document","source":{"type":"base64","media_type":"SECRET base64 media type","data":"SECRET base64 source"}},{"type":"document","title":"SECRET custom content title","source":{"type":"content","url":"SECRET custom content URL","cache_control":{"type":"SECRET custom content cache control"},"content":[{"type":"text","text":"SECRET custom content first","citation":{"url":"SECRET custom content citation URL","title":"SECRET custom content citation title"}},{"type":"image","source":{"data":"SECRET custom content image"},"title":"SECRET custom content image title"},{"type":"text","text":"SECRET custom content second","cache_control":{"type":"SECRET custom content part cache control"}}]}},{"type":"tool_result","tool_use_id":"SECRET tool use id","is_error":"SECRET is error","content":["SECRET tool result",{"type":"search_result","source":"SECRET nested search source","title":"SECRET nested search title","citations":[{"data":"SECRET nested citation data"}],"content":[{"type":"text","text":"SECRET nested search"},{"type":"image","source":{"data":"SECRET nested search non-text"}}]},{"type":"document","title":"SECRET nested document title","source":{"type":"text","media_type":"SECRET nested document media type","data":"SECRET nested document"}},{"type":"document","source":{"type":"url","url":"SECRET nested URL source"}},{"type":"document","source":{"type":"file","file_id":"SECRET nested file source"}},{"type":"document","source":{"type":"base64","media_type":"SECRET nested base64 media type","data":"SECRET nested base64 source"}}]},{"type":"tool_use","input":{"query":"SECRET tool input"}},{"type":"thinking","thinking":"SECRET thinking","text":"SECRET thinking text"},{"type":"redacted_thinking","data":"SECRET redacted thinking"},{"type":"image","source":{"data":"SECRET image source"}}]}]}`)
	resp := interceptRPC(t, "claude", body)
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET direct search"`, After: `" direct search"`},
		rawReplacement{Before: `"SECRET direct document"`, After: `" direct document"`},
		rawReplacement{Before: `"SECRET custom content first"`, After: `" custom content first"`},
		rawReplacement{Before: `"SECRET custom content second"`, After: `" custom content second"`},
		rawReplacement{Before: `"SECRET nested search"`, After: `" nested search"`},
		rawReplacement{Before: `"SECRET nested document"`, After: `" nested document"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside selected Claude result text")
	}
}
