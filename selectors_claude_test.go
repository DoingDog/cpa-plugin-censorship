package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/tidwall/gjson"
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
		rawReplacement{Before: `"SECRET document"`, After: `" document"`},
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

func TestClaudeCompactionInstructionsUseSystemScope(t *testing.T) {
	body := []byte(`{"context_management":{"edits":[{"type":"compact_20260112","instructions":"SECRET instructions","trigger":{"type":"input_tokens","value":1000},"pause_after_compaction":true},{"type":"future","instructions":"SECRET future"},{"type":"compact_20260112","instructions":null}]}}`)
	want := []byte(`{"context_management":{"edits":[{"type":"compact_20260112","instructions":" instructions","trigger":{"type":"input_tokens","value":1000},"pause_after_compaction":true},{"type":"future","instructions":"SECRET future"},{"type":"compact_20260112","instructions":null}]}}`)

	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system]\n")
	resp := interceptRPC(t, "claude", body)
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v body = %s want = %s", resp, resp.Body, want)
	}

	assertBlockedRole(t, "claude", string(body), "system")

	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	resp = interceptRPC(t, "claude", body)
	if resp.Terminate || resp.Body != nil {
		t.Fatalf("user-only response = %#v", resp)
	}
}

func TestClaudeBetaMCPToolResult(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"mcp_tool_result","tool_use_id":"SECRET id","is_error":false,"content":"SECRET scalar","name":"SECRET name"},{"type":"mcp_tool_result","content":[{"type":"text","text":"SECRET typed"},{"type":"image","source":{"data":"SECRET image"}},{"type":"document","title":"SECRET document"},{"type":"unknown","text":"SECRET unknown"},{"text":"SECRET missing type"},{"type":null,"text":"SECRET null type"},{"type":42,"text":"SECRET numeric type"},{"type":"text","text":null},{"type":"text","text":42}]},{"type":"mcp_tool_result","content":{"text":"SECRET object"}},{"type":"mcp_tool_result","content":null},{"type":"mcp_tool_result","content":42}]}]}`)
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET scalar"`, After: `" scalar"`},
		rawReplacement{Before: `"SECRET typed"`, After: `" typed"`},
	)

	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [tool]\n")
	resp := interceptRPC(t, "claude", body)
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v body = %s want = %s", resp, resp.Body, want)
	}

	assertBlockedRole(t, "claude", string(body), "tool")

	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	resp = interceptRPC(t, "claude", body)
	if resp.Terminate || resp.Body != nil {
		t.Fatalf("user-only response = %#v", resp)
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
		{
			name:          "direct user search and document metadata",
			roles:         "[user]",
			disabledRoles: "[tool]",
			body:          `{"messages":[{"role":"user","content":[{"type":"search_result","source":"SECRET search source","title":"SECRET search title","content":[{"type":"text","text":"SECRET search text"}]},{"type":"document","title":"SECRET document title","context":"SECRET document context","source":{"type":"content","content":"SECRET scalar document"}}]}]}`,
			replacements: []rawReplacement{
				{Before: `"SECRET search title"`, After: `" search title"`},
				{Before: `"SECRET search text"`, After: `" search text"`},
				{Before: `"SECRET document title"`, After: `" document title"`},
				{Before: `"SECRET document context"`, After: `" document context"`},
				{Before: `"SECRET scalar document"`, After: `" scalar document"`},
			},
		},
		{
			name:          "nested tool search and document metadata",
			roles:         "[tool]",
			disabledRoles: "[user]",
			body:          `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"SECRET tool id","content":[{"type":"search_result","source":"SECRET nested source","title":"SECRET nested search title","content":[{"type":"text","text":"SECRET nested search text"}]},{"type":"document","title":"SECRET nested document title","context":"SECRET nested document context","source":{"type":"content","content":"SECRET nested scalar document"}}]}]}]}`,
			replacements: []rawReplacement{
				{Before: `"SECRET nested search title"`, After: `" nested search title"`},
				{Before: `"SECRET nested search text"`, After: `" nested search text"`},
				{Before: `"SECRET nested document title"`, After: `" nested document title"`},
				{Before: `"SECRET nested document context"`, After: `" nested document context"`},
				{Before: `"SECRET nested scalar document"`, After: `" nested scalar document"`},
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
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"search_result","source":"SECRET search source","title":"SECRET search title","citations":[{"data":"SECRET citation data"}],"content":[{"type":"text","text":"SECRET direct search"},{"type":"image","source":{"data":"SECRET search non-text"}}]},{"type":"document","title":"SECRET document title","context":"SECRET direct document context","source":{"type":"text","media_type":"SECRET document media type","data":"SECRET direct document"}},{"type":"document","context":"SECRET direct URL document context","source":{"type":"url","url":"SECRET URL source"}},{"type":"document","context":"SECRET direct file document context","source":{"type":"file","file_id":"SECRET file source"}},{"type":"document","context":"SECRET direct base64 document context","source":{"type":"base64","media_type":"SECRET base64 media type","data":"SECRET base64 source"}},{"type":"document","title":"SECRET custom content title","context":"SECRET direct custom content document context","source":{"type":"content","url":"SECRET custom content URL","cache_control":{"type":"SECRET custom content cache control"},"content":[{"type":"text","text":"SECRET custom content first","citation":{"url":"SECRET custom content citation URL","title":"SECRET custom content citation title"}},{"type":"image","source":{"data":"SECRET custom content image"},"title":"SECRET custom content image title"},{"type":"text","text":"SECRET custom content second","cache_control":{"type":"SECRET custom content part cache control"}}]}},{"type":"document","title":"SECRET direct scalar content document title","context":"SECRET direct scalar content document context","source":{"type":"content","content":"SECRET direct scalar document"}},{"type":"tool_result","tool_use_id":"SECRET tool use id","is_error":"SECRET is error","content":["SECRET tool result",{"type":"search_result","source":"SECRET nested search source","title":"SECRET nested search title","citations":[{"data":"SECRET nested citation data"}],"content":[{"type":"text","text":"SECRET nested search"},{"type":"image","source":{"data":"SECRET nested search non-text"}}]},{"type":"document","title":"SECRET nested document title","context":"SECRET nested document context","source":{"type":"text","media_type":"SECRET nested document media type","data":"SECRET nested document"}},{"type":"document","context":"SECRET nested URL document context","source":{"type":"url","url":"SECRET nested URL source"}},{"type":"document","context":"SECRET nested file document context","source":{"type":"file","file_id":"SECRET nested file source"}},{"type":"document","context":"SECRET nested base64 document context","source":{"type":"base64","media_type":"SECRET nested base64 media type","data":"SECRET nested base64 source"}},{"type":"document","title":"SECRET nested scalar content document title","context":"SECRET nested scalar content document context","source":{"type":"content","content":"SECRET nested scalar document"}}]},{"type":"tool_use","input":{"query":"SECRET tool input"}},{"type":"thinking","thinking":"SECRET thinking","text":"SECRET thinking text"},{"type":"redacted_thinking","data":"SECRET redacted thinking"},{"type":"image","source":{"data":"SECRET image source"}}]}]}`)
	resp := interceptRPC(t, "claude", body)
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET search title"`, After: `" search title"`},
		rawReplacement{Before: `"SECRET direct search"`, After: `" direct search"`},
		rawReplacement{Before: `"SECRET document title"`, After: `" document title"`},
		rawReplacement{Before: `"SECRET direct document context"`, After: `" direct document context"`},
		rawReplacement{Before: `"SECRET direct document"`, After: `" direct document"`},
		rawReplacement{Before: `"SECRET direct URL document context"`, After: `" direct URL document context"`},
		rawReplacement{Before: `"SECRET direct file document context"`, After: `" direct file document context"`},
		rawReplacement{Before: `"SECRET direct base64 document context"`, After: `" direct base64 document context"`},
		rawReplacement{Before: `"SECRET custom content title"`, After: `" custom content title"`},
		rawReplacement{Before: `"SECRET direct custom content document context"`, After: `" direct custom content document context"`},
		rawReplacement{Before: `"SECRET custom content first"`, After: `" custom content first"`},
		rawReplacement{Before: `"SECRET custom content second"`, After: `" custom content second"`},
		rawReplacement{Before: `"SECRET direct scalar content document title"`, After: `" direct scalar content document title"`},
		rawReplacement{Before: `"SECRET direct scalar content document context"`, After: `" direct scalar content document context"`},
		rawReplacement{Before: `"SECRET direct scalar document"`, After: `" direct scalar document"`},
		rawReplacement{Before: `"SECRET nested search title"`, After: `" nested search title"`},
		rawReplacement{Before: `"SECRET nested search"`, After: `" nested search"`},
		rawReplacement{Before: `"SECRET nested document title"`, After: `" nested document title"`},
		rawReplacement{Before: `"SECRET nested document context"`, After: `" nested document context"`},
		rawReplacement{Before: `"SECRET nested document"`, After: `" nested document"`},
		rawReplacement{Before: `"SECRET nested URL document context"`, After: `" nested URL document context"`},
		rawReplacement{Before: `"SECRET nested file document context"`, After: `" nested file document context"`},
		rawReplacement{Before: `"SECRET nested base64 document context"`, After: `" nested base64 document context"`},
		rawReplacement{Before: `"SECRET nested scalar content document title"`, After: `" nested scalar content document title"`},
		rawReplacement{Before: `"SECRET nested scalar content document context"`, After: `" nested scalar content document context"`},
		rawReplacement{Before: `"SECRET nested scalar document"`, After: `" nested scalar document"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside selected Claude result text")
	}
}

func TestClaudeMinimumLengthFieldsRejectFullStrip(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{name: "typed text", body: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET"}]}]}`)},
		{name: "document title", body: []byte(`{"messages":[{"role":"user","content":[{"type":"document","title":"SECRET","source":{"type":"text","data":"clean"}}]}]}`)},
		{name: "document context", body: []byte(`{"messages":[{"role":"user","content":[{"type":"document","context":"SECRET","source":{"type":"text","data":"clean"}}]}]}`)},
		{name: "scalar user content", body: []byte(`{"messages":[{"role":"user","content":"SECRET"}]}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
			resp := interceptRPC(t, "claude", tc.body)
			if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || resp.ResponseHeaders.Get("Content-Type") != "application/json" || len(resp.Body) != 0 {
				t.Fatalf("response = %#v", resp)
			}
			if got := gjson.GetBytes(resp.ResponseBody, "error.code").String(); got != "censorship_invalid_request" {
				t.Fatalf("error code = %q, body = %s", got, resp.ResponseBody)
			}
			if got := gjson.GetBytes(resp.ResponseBody, "error.message").String(); got != "censorship rewrite would make a text field invalid" {
				t.Fatalf("error message = %q, body = %s", got, resp.ResponseBody)
			}
		})
	}
}

func TestClaudeMinimumLengthFieldsAllowPartialStrip(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		want []byte
	}{
		{
			name: "typed text",
			body: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET tail"}]}]}`),
			want: []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":" tail"}]}]}`),
		},
		{
			name: "document title",
			body: []byte(`{"messages":[{"role":"user","content":[{"type":"document","title":"SECRET title","source":{"type":"text","data":"clean"}}]}]}`),
			want: []byte(`{"messages":[{"role":"user","content":[{"type":"document","title":" title","source":{"type":"text","data":"clean"}}]}]}`),
		},
		{
			name: "document context",
			body: []byte(`{"messages":[{"role":"user","content":[{"type":"document","context":"SECRET context","source":{"type":"text","data":"clean"}}]}]}`),
			want: []byte(`{"messages":[{"role":"user","content":[{"type":"document","context":" context","source":{"type":"text","data":"clean"}}]}]}`),
		},
		{
			name: "scalar user content",
			body: []byte(`{"messages":[{"role":"user","content":"before SECRET after"}],"decoy":"SECRET"}`),
			want: []byte(`{"messages":[{"role":"user","content":"before  after"}],"decoy":"SECRET"}`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
			resp := interceptRPC(t, "claude", tc.body)
			if resp.Terminate || !bytes.Equal(resp.Body, tc.want) {
				t.Fatalf("response = %#v, want body %s", resp, tc.want)
			}
		})
	}
}

func TestClaudeScalarUserContentControls(t *testing.T) {
	t.Run("block reports user role", func(t *testing.T) {
		assertBlockedRole(t, "claude", `{"messages":[{"role":"user","content":"SECRET"}]}`, "user")
	})
	t.Run("obfs returns non-empty transformed text", func(t *testing.T) {
		registerConfig(t, "words:\n  obfs: [SECRET]\nscope:\n  roles: [user]\n")
		resp := interceptRPC(t, "claude", []byte(`{"messages":[{"role":"user","content":"SECRET"}]}`))
		content := gjson.GetBytes(resp.Body, "messages.0.content")
		if resp.Terminate || content.Type != gjson.String || content.Str == "" || content.Str == "SECRET" {
			t.Fatalf("response = %#v", resp)
		}
	})
}

func TestClaudeNonUserScalarContentAllowsFullStrip(t *testing.T) {
	cases := []struct {
		name, roles string
		body, want  []byte
	}{
		{
			name:  "top-level system",
			roles: "[system]",
			body:  []byte(`{"system":"SECRET"}`),
			want:  []byte(`{"system":""}`),
		},
		{
			name:  "assistant prefill",
			roles: "[assistant]",
			body:  []byte(`{"messages":[{"role":"assistant","content":"SECRET"}]}`),
			want:  []byte(`{"messages":[{"role":"assistant","content":""}]}`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: "+tc.roles+"\n")
			resp := interceptRPC(t, "claude", tc.body)
			if resp.Terminate || !bytes.Equal(resp.Body, tc.want) {
				t.Fatalf("response = %#v, want body %s", resp, tc.want)
			}
		})
	}
}
