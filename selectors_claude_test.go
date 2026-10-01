package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
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

func TestClaudeAssistantMCPHistory(t *testing.T) {
	for _, shape := range []struct {
		name, content string
	}{
		{"scalar", `"SECRET tail"`},
		{"typed", `[{"type":"text","text":"SECRET tail","cache_control":{"type":"ephemeral"}},{"type":"image","source":{"type":"base64","data":"SECRET image"}},{"type":"unknown","text":"SECRET unknown"}]`},
	} {
		for _, action := range []struct {
			name, after string
		}{
			{"block", ""},
			{"strip", `" tail"`},
			{"obfs", "\"S​ECRET tail\""},
		} {
			for _, scope := range []struct {
				name, roles string
			}{
				{"tool", "[tool]"},
				{"assistant_tool", "[assistant, tool]"},
			} {
				t.Run(shape.name+"/"+action.name+"/"+scope.name, func(t *testing.T) {
					registerConfig(t, "words:\n  "+action.name+": [SECRET]\nscope:\n  roles: "+scope.roles+"\n")
					body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"safe"},{"role":"assistant","content":[{"type":"mcp_tool_use","id":"SECRET id","name":"SECRET name","server_name":"SECRET server","input":{"query":"SECRET input"}},{"type":"mcp_tool_result","tool_use_id":"SECRET id","name":"SECRET name","server_name":"SECRET server","is_error":false,"cache_control":{"type":"ephemeral"},"unknown":"SECRET unknown","content":` + shape.content + `}]},{"role":"user","content":"continue"}]}`)
					resp := interceptRPC(t, "claude", body)
					if action.name == "block" {
						if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || resp.Body != nil || gjson.GetBytes(resp.ResponseBody, "error.code").Str != "censorship_blocked" || gjson.GetBytes(resp.ResponseBody, "error.term").Str != "SECRET" || gjson.GetBytes(resp.ResponseBody, "error.role").Str != "tool" {
							t.Fatalf("want canonical tool block, terminate=%t status=%d error=%s", resp.Terminate, resp.StatusCode, resp.ResponseBody)
						}
						return
					}
					want := replaceRawTokens(t, body, rawReplacement{Before: `"SECRET tail"`, After: action.after})
					if resp.Terminate || !bytes.Equal(resp.Body, want) {
						t.Fatalf("body differs from the single expected MCP text rewrite, terminate=%t", resp.Terminate)
					}
				})
			}
		}
	}
}

func TestClaudeAssistantMCPHistoryNonEmpty(t *testing.T) {
	for _, role := range []string{"user", "assistant"} {
		for _, shape := range []struct {
			name, content string
			invalid       bool
		}{
			{"scalar", `"SECRET"`, false},
			{"typed", `[{"type":"text","text":"SECRET"}]`, true},
		} {
			t.Run(role+"/"+shape.name, func(t *testing.T) {
				registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [tool]\n")
				body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"safe"},{"role":"` + role + `","content":[{"type":"mcp_tool_result","tool_use_id":"call-1","content":` + shape.content + `}]},{"role":"user","content":"continue"}]}`)
				resp := interceptRPC(t, "claude", body)
				if shape.invalid {
					if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || resp.Body != nil || gjson.GetBytes(resp.ResponseBody, "error.code").Str != "censorship_invalid_request" {
						t.Fatalf("want local invalid typed text, response=%#v", resp)
					}
					return
				}
				want := replaceRawTokens(t, body, rawReplacement{Before: `"SECRET"`, After: `""`})
				if resp.Terminate || !bytes.Equal(resp.Body, want) {
					t.Fatalf("want empty scalar MCP result, response=%#v", resp)
				}
			})
		}
	}
}

func TestClaudeAssistantMCPHistoryScopeControls(t *testing.T) {
	for _, tc := range []struct {
		name, roles, message string
	}{
		{"tool disabled scalar", "[assistant]", `{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"call-1","content":"SECRET"}]}`},
		{"tool disabled typed", "[assistant]", `{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"call-1","content":[{"type":"text","text":"SECRET"}]}]}`},
		{"ordinary assistant scalar tool-only", "[tool]", `{"role":"assistant","content":"SECRET"}`},
		{"ordinary assistant typed tool-only", "[tool]", `{"role":"assistant","content":[{"type":"text","text":"SECRET"}]}`},
		{"ordinary assistant tool_result tool-only", "[tool]", `{"role":"assistant","content":[{"type":"tool_result","tool_use_id":"call-1","content":"SECRET"},{"type":"tool_result","content":[{"type":"text","text":"SECRET"}]}]}`},
		{"ordinary assistant tool_result own and tool", "[assistant, tool]", `{"role":"assistant","content":[{"type":"tool_result","tool_use_id":"call-1","content":"SECRET"},{"type":"tool_result","content":[{"type":"text","text":"SECRET"}]}]}`},
		{"system MCP excluded", "[system, tool]", `{"role":"system","content":[{"type":"mcp_tool_result","content":"SECRET"}]}`},
		{"system own scope disabled", "[tool]", `{"role":"system","content":"SECRET"}`},
		{"unsupported role MCP excluded", "[tool]", `{"role":"developer","content":[{"type":"mcp_tool_result","content":"SECRET"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: "+tc.roles+"\n")
			body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"safe"},` + tc.message + `,{"role":"user","content":"continue"}]}`)
			requireNoOpResponse(t, interceptRPC(t, "claude", body))
		})
	}
	t.Run("ordinary assistant text keeps own scope", func(t *testing.T) {
		registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [assistant]\n")
		body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"text","text":"SECRET own"},{"type":"mcp_tool_result","tool_use_id":"SECRET id","content":"SECRET MCP"}]}]}`)
		want := replaceRawTokens(t, body, rawReplacement{Before: `"SECRET own"`, After: `" own"`})
		resp := interceptRPC(t, "claude", body)
		if resp.Terminate || !bytes.Equal(resp.Body, want) {
			t.Fatalf("ordinary assistant own scope changed MCP text, response=%#v", resp)
		}
	})
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

func TestClaudeMCPNonEmpty(t *testing.T) {
	config := "words:\n  strip: [SECRET]\nscope:\n  roles: [tool]\n"
	t.Run("full strip rejects empty typed text", func(t *testing.T) {
		registerConfig(t, config)
		body := []byte(`{"messages":[{"role":"user","content":[{"type":"mcp_tool_result","content":[{"type":"text","text":"SECRET"}]}]}]}`)
		resp := interceptRPC(t, "claude", body)
		if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || len(resp.Body) != 0 {
			t.Fatalf("response = %#v", resp)
		}
		if got := gjson.GetBytes(resp.ResponseBody, "error.code").String(); got != "censorship_invalid_request" {
			t.Fatalf("error code = %q, body = %s", got, resp.ResponseBody)
		}
		if got := gjson.GetBytes(resp.ResponseBody, "error.message").String(); got != "censorship rewrite would make a text field invalid" {
			t.Fatalf("error message = %q, body = %s", got, resp.ResponseBody)
		}
	})
	t.Run("partial strip keeps typed text", func(t *testing.T) {
		registerConfig(t, config)
		body := []byte(`{"messages":[{"role":"user","content":[{"type":"mcp_tool_result","content":[{"type":"text","text":"SECRET tail"}]}]}]}`)
		want := []byte(`{"messages":[{"role":"user","content":[{"type":"mcp_tool_result","content":[{"type":"text","text":" tail"}]}]}]}`)
		resp := interceptRPC(t, "claude", body)
		if resp.Terminate || !bytes.Equal(resp.Body, want) {
			t.Fatalf("response = %#v, want body %s", resp, want)
		}
	})
	t.Run("untyped inner block is untouched", func(t *testing.T) {
		registerConfig(t, config)
		body := []byte(`{"messages":[{"role":"user","content":[{"type":"mcp_tool_result","content":[{"type":"image","text":"SECRET"}]}]}]}`)
		resp := interceptRPC(t, "claude", body)
		if resp.Terminate || resp.Body != nil {
			t.Fatalf("response = %#v", resp)
		}
	})
	t.Run("user role leaves typed text untouched", func(t *testing.T) {
		registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [user]\n")
		body := []byte(`{"messages":[{"role":"user","content":[{"type":"mcp_tool_result","content":[{"type":"text","text":"SECRET"}]}]}]}`)
		resp := interceptRPC(t, "claude", body)
		if resp.Terminate || resp.Body != nil {
			t.Fatalf("response = %#v", resp)
		}
	})
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

func TestClaudeDocumentTitleMaxCodePoints(t *testing.T) {
	const unicodeEscape = "\\u"
	for _, path := range []struct {
		name, role, prefix, suffix string
	}{
		{"direct user", "user", `{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":[`, `]}]}`},
		{"nested tool", "tool", `{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"ab id","is_error":false,"content":[`, `]}]}]}`},
	} {
		for _, tc := range []struct {
			name, title, rawTitle, rules, want, wantCode string
			noOp, disabled                               bool
		}{
			{name: "ASCII final 500", title: strings.Repeat("x", 497) + "ab", want: strings.Repeat("x", 497) + "a​b"},
			{name: "ASCII final 501", title: strings.Repeat("x", 498) + "ab", wantCode: "censorship_invalid_request"},
			{name: "Chinese final 500", title: strings.Repeat("中", 497) + "ab", want: strings.Repeat("中", 497) + "a​b"},
			{name: "Chinese final 501", title: strings.Repeat("中", 498) + "ab", wantCode: "censorship_invalid_request"},
			{name: "emoji final 500", title: strings.Repeat("😀", 497) + "ab", want: strings.Repeat("😀", 497) + "a​b"},
			{name: "emoji final 501", title: strings.Repeat("😀", 498) + "ab", wantCode: "censorship_invalid_request"},
			{name: "escaped ASCII final 500", rawTitle: `"` + strings.Repeat("x", 497) + unicodeEscape + "0061" + unicodeEscape + `0062"`, want: strings.Repeat("x", 497) + "a​b"},
			{name: "escaped ASCII final 501", rawTitle: `"` + strings.Repeat("x", 498) + unicodeEscape + "0061" + unicodeEscape + `0062"`, wantCode: "censorship_invalid_request"},
			{name: "escaped Chinese final 500", rawTitle: `"` + strings.Repeat(unicodeEscape+"4e2d", 497) + `ab"`, want: strings.Repeat("中", 497) + "a​b"},
			{name: "escaped Chinese final 501", rawTitle: `"` + strings.Repeat(unicodeEscape+"4e2d", 498) + `ab"`, wantCode: "censorship_invalid_request"},
			{name: "surrogate pairs final 500", rawTitle: `"` + strings.Repeat(unicodeEscape+"d83d"+unicodeEscape+"de00", 497) + `ab"`, want: strings.Repeat("😀", 497) + "a​b"},
			{name: "surrogate pairs final 501", rawTitle: `"` + strings.Repeat(unicodeEscape+"d83d"+unicodeEscape+"de00", 498) + `ab"`, wantCode: "censorship_invalid_request"},
			{name: "strip then obfs final 500", title: strings.Repeat("x", 494) + "zzabab", rules: "  strip: [zz]\n  obfs: [ab]\n", want: strings.Repeat("x", 494) + "a​ba​b"},
			{name: "strip then obfs final 501", title: strings.Repeat("x", 495) + "zabab", rules: "  strip: [z]\n  obfs: [ab]\n", wantCode: "censorship_invalid_request"},
			{name: "cancellation preserves escaped token", rawTitle: `"` + strings.Repeat("x", 497) + "a" + unicodeEscape + `200bb"`, rules: "  strip: ['​']\n  obfs: [ab]\n", noOp: true},
			{name: "block precedes invalid expansion", title: strings.Repeat("x", 498) + "ab", rules: "  block: [ab]\n  strip: [ab]\n  obfs: [ab]\n", wantCode: "censorship_blocked"},
			{name: "scope disabled", title: strings.Repeat("x", 498) + "ab", noOp: true, disabled: true},
			{name: "title retains nonempty constraint", title: "ab", rules: "  strip: [ab]\n", wantCode: "censorship_invalid_request"},
		} {
			t.Run(path.name+"/"+tc.name, func(t *testing.T) {
				rules := tc.rules
				if rules == "" {
					rules = "  obfs: [ab]\n"
				}
				role := path.role
				if tc.disabled {
					role = "assistant"
				}
				registerConfig(t, "words:\n"+rules+"scope:\n  roles: ["+role+"]\n")
				title, err := json.Marshal(tc.title)
				if err != nil {
					t.Fatal(err)
				}
				if tc.rawTitle != "" {
					title = []byte(tc.rawTitle)
				}
				body := []byte(path.prefix + `{"type":"document","title":` + string(title) + `,"context":"clean context","source":{"type":"text","data":"clean"},"cache_control":{"type":"ephemeral","unknown":"ab"},"unknown":"ab"}` + path.suffix)
				resp := interceptRPC(t, "claude", body)
				if tc.wantCode != "" {
					if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || resp.Body != nil || gjson.GetBytes(resp.ResponseBody, "error.code").Str != tc.wantCode {
						t.Fatalf("want %s, terminate=%t status=%d body bytes=%d error=%s", tc.wantCode, resp.Terminate, resp.StatusCode, len(resp.Body), resp.ResponseBody)
					}
					if tc.wantCode == "censorship_blocked" && (gjson.GetBytes(resp.ResponseBody, "error.role").Str != path.role || gjson.GetBytes(resp.ResponseBody, "error.term").Str != "ab") {
						t.Fatalf("want canonical %s block for ab, error=%s", path.role, resp.ResponseBody)
					}
					return
				}
				if tc.noOp {
					requireNoOpResponse(t, resp)
					return
				}
				wantTitle, err := json.Marshal(tc.want)
				if err != nil {
					t.Fatal(err)
				}
				want := replaceRawTokens(t, body, rawReplacement{Before: string(title), After: string(wantTitle)})
				if resp.Terminate || !bytes.Equal(resp.Body, want) {
					t.Fatalf("body differs from expected title rewrite, terminate=%t error=%s", resp.Terminate, resp.ResponseBody)
				}
			})
		}
	}
}

func TestClaudeDocumentTitleDoesNotLimitOtherResultText(t *testing.T) {
	value, err := json.Marshal(strings.Repeat("x", 498) + "ab")
	if err != nil {
		t.Fatal(err)
	}
	wantValue, err := json.Marshal(strings.Repeat("x", 498) + "a​b")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []struct {
		name, role, prefix, suffix string
	}{
		{"direct user", "user", `{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":[`, `]}]}`},
		{"nested tool", "tool", `{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"ab id","content":[`, `]}]}]}`},
	} {
		for _, tc := range []struct {
			name, prefix, suffix string
		}{
			{"document context", `{"type":"document","title":"clean","context":`, `,"source":{"type":"text","data":"clean"}}`},
			{"document context absent title", `{"type":"document","context":`, `,"source":{"type":"text","data":"clean"}}`},
			{"document context empty title", `{"type":"document","title":"","context":`, `,"source":{"type":"text","data":"clean"}}`},
			{"document context nonstring title", `{"type":"document","title":42,"context":`, `,"source":{"type":"text","data":"clean"}}`},
			{"search_result title", `{"type":"search_result","title":`, `,"source":"ab machine","content":[{"type":"text","text":"clean"}]}`},
		} {
			t.Run(path.name+"/"+tc.name, func(t *testing.T) {
				registerConfig(t, "words:\n  obfs: [ab]\nscope:\n  roles: ["+path.role+"]\n")
				body := []byte(path.prefix + tc.prefix + string(value) + tc.suffix + path.suffix)
				want := replaceRawTokens(t, body, rawReplacement{Before: string(value), After: string(wantValue)})
				resp := interceptRPC(t, "claude", body)
				if resp.Terminate || !bytes.Equal(resp.Body, want) {
					t.Fatalf("want unbounded 501-codepoint result text, terminate=%t error=%s", resp.Terminate, resp.ResponseBody)
				}
			})
		}
	}
}

func TestClaudeDocumentTitlePreservesUnchangedToken(t *testing.T) {
	const unicodeEscape = "\\u"
	for _, path := range []struct {
		name, role, prefix, suffix string
	}{
		{"direct user", "user", `{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":[`, `]}]}`},
		{"nested tool", "tool", `{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"CHANGE id","is_error":false,"content":[`, `]}]}]}`},
	} {
		for _, tc := range []struct {
			name, title, rules string
		}{
			{"unmatched escaped title", `"` + strings.Repeat(unicodeEscape+"0078", 498) + `ab"`, "  strip: [CHANGE]\n"},
			{"cancelled escaped title", `"` + strings.Repeat("x", 497) + "a" + unicodeEscape + `200bb"`, "  strip: ['​', CHANGE]\n  obfs: [ab]\n"},
		} {
			t.Run(path.name+"/"+tc.name, func(t *testing.T) {
				registerConfig(t, "words:\n"+tc.rules+"scope:\n  roles: ["+path.role+"]\n")
				body := []byte(path.prefix + `{"type":"document","title":` + tc.title + `,"context":"CHANGE context","source":{"type":"url","url":"CHANGE machine"},"unknown":"CHANGE metadata"}` + path.suffix)
				want := replaceRawTokens(t, body, rawReplacement{Before: `"CHANGE context"`, After: `" context"`})
				resp := interceptRPC(t, "claude", body)
				if resp.Terminate || !bytes.Equal(resp.Body, want) {
					t.Fatalf("unchanged title or machine token was rewritten, terminate=%t error=%s", resp.Terminate, resp.ResponseBody)
				}
			})
		}
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

func TestClaudeDescriptionPaths(t *testing.T) {
	cases := []struct {
		name, body string
	}{
		{"tool description", `{"tools":[{"name":"lookup","description":"SECRET lookup","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"safe"}]}`},
		{"tool schema description", `{"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"query":{"type":"array","items":{"type":"string","description":"SECRET query"}}}}}],"messages":[{"role":"user","content":"safe"}]}`},
		{"output schema description", `{"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET answer"}}}}},"messages":[{"role":"user","content":"safe"}]}`},
		{"output_format", `{"output_format":{"type":"json_schema","schema":{"type":"object","properties":{"answer":{"type":"string","description":"SECRET answer"}}}},"messages":[{"role":"user","content":"safe"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [system]\n")
			resp := interceptRPC(t, "claude", []byte(tc.body))
			if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || gjson.GetBytes(resp.ResponseBody, "error.term").String() != "SECRET" || gjson.GetBytes(resp.ResponseBody, "error.role").String() != "system" {
				t.Fatalf("response = %#v body = %s", resp, resp.ResponseBody)
			}

			registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [user]\n")
			resp = interceptRPC(t, "claude", []byte(tc.body))
			if resp.Terminate || resp.Body != nil {
				t.Fatalf("user-only response = %#v", resp)
			}
		})
	}
}

func TestClaudeDescriptionRewrites(t *testing.T) {
	body := []byte(`{"tools":[{"name":"SECRET_tool","description":"SECRET tool","input_schema":{"type":"object","properties":{"SECRET_field":{"type":"string","description":"SECRET input","enum":["SECRET"],"const":"SECRET"}}},"input_examples":[{"SECRET_field":"SECRET"}]}],"output_config":{"format":{"type":"json_schema","schema":{"type":"object","properties":{"SECRET_answer":{"type":"string","description":"SECRET output","enum":["SECRET"]}}}}},"messages":[{"role":"user","content":"safe"}]}`)
	registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [system]\n")
	resp := interceptRPC(t, "claude", body)
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET tool"`, After: `" tool"`},
		rawReplacement{Before: `"SECRET input"`, After: `" input"`},
		rawReplacement{Before: `"SECRET output"`, After: `" output"`},
	)
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v body = %s, want = %s", resp, resp.Body, want)
	}

	t.Run("legacy output_format changes only description", func(t *testing.T) {
		body := []byte(`{"output_format":{"type":"json_schema","schema":{"type":"object","properties":{"SECRET_answer":{"type":"string","description":"SECRET output","enum":["SECRET"],"default":"SECRET"}}}},"messages":[{"role":"user","content":"safe"}]}`)
		want := replaceRawTokens(t, body, rawReplacement{Before: `"SECRET output"`, After: `" output"`})
		resp := interceptRPC(t, "claude", body)
		if resp.Terminate || !bytes.Equal(resp.Body, want) {
			t.Fatalf("response = %#v body = %s, want = %s", resp, resp.Body, want)
		}
	})

	t.Run("full strip keeps optional description", func(t *testing.T) {
		registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [system]\n")
		resp := interceptRPC(t, "claude", []byte(`{"tools":[{"name":"lookup","description":"SECRET","input_schema":{"type":"object"}}]}`))
		if resp.Terminate || string(resp.Body) != `{"tools":[{"name":"lookup","description":"","input_schema":{"type":"object"}}]}` {
			t.Fatalf("response = %#v", resp)
		}
	})
	t.Run("obfs changes only description", func(t *testing.T) {
		registerConfig(t, "words:\n  obfs: [SECRET]\nscope:\n  roles: [system]\n")
		resp := interceptRPC(t, "claude", []byte(`{"tools":[{"name":"SECRET_tool","description":"SECRET","input_schema":{"type":"object"}}]}`))
		if resp.Terminate || gjson.GetBytes(resp.Body, "tools.0.name").Str != "SECRET_tool" || gjson.GetBytes(resp.Body, "tools.0.description").Str != "S​ECRET" {
			t.Fatalf("response = %#v", resp)
		}
	})
	t.Run("unsupported shapes stay unchanged", func(t *testing.T) {
		registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [system]\n")
		for _, body := range []string{
			`{"tools":[{"name":"lookup","description":"SECRET"}]}`,
			`{"tools":[{"description":"SECRET","input_schema":{"type":"object"}}]}`,
			`{"tools":[{"type":"web_search_20260209","name":"web_search","description":"SECRET"}]}`,
			`{"tools":{"description":"SECRET"}}`,
			`{"tools":[{"name":"lookup","description":42,"input_schema":{"type":"object","properties":{"query":{"enum":["SECRET"]}}}}]}`,
			`{"output_config":{"format":{"type":"text","schema":{"description":"SECRET"}}}}`,
			`{"output_config":{"format":{"type":null,"schema":{"description":"SECRET"}}}}`,
			`{"output_format":{"type":"text","schema":{"description":"SECRET"}}}`,
		} {
			resp := interceptRPC(t, "claude", []byte(body))
			if resp.Terminate || resp.Body != nil {
				t.Fatalf("body = %s response = %#v", body, resp)
			}
		}
	})
}

func TestClaudeHistoryAssistantScalarNonEmpty(t *testing.T) {
	registerConfig(t, "words:\n  strip: [Hello.]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"Greet me."},{"role":"assistant","content":"Hello."},{"role":"user","content":"Continue."}]}`)
	resp := interceptRPC(t, "claude", body)
	if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || resp.Body != nil || gjson.GetBytes(resp.ResponseBody, "error.code").Str != "censorship_invalid_request" {
		t.Fatalf("want local invalid history, terminate=%t status=%d error=%s", resp.Terminate, resp.StatusCode, resp.ResponseBody)
	}
}

func TestClaudeHistoryAssistantScalarControls(t *testing.T) {
	for _, tc := range []struct {
		name, roles, messages string
		invalid               bool
		replacements          []rawReplacement
	}{
		{"last scalar", "[assistant]", `[{"role":"user","content":"Greet me."},{"role":"assistant","content":"Hello."}]`, false, []rawReplacement{{Before: `"Hello."`, After: `""`}}},
		{"existing empty prefill", "[assistant]", `[{"role":"user","content":"Greet me."},{"role":"assistant","content":""}]`, false, nil},
		{"partial history", "[assistant]", `[{"role":"user","content":"Greet me."},{"role":"assistant","content":"Hello. tail"},{"role":"user","content":"Continue."}]`, false, []rawReplacement{{Before: `"Hello. tail"`, After: `" tail"`}}},
		{"adjacent earlier scalar", "[assistant]", `[{"role":"user","content":"Greet me."},{"role":"assistant","content":"Hello."},{"role":"assistant","content":"Later."}]`, true, nil},
		{"adjacent last scalar", "[assistant]", `[{"role":"user","content":"Greet me."},{"role":"assistant","content":"Earlier."},{"role":"assistant","content":"Hello."}]`, false, []rawReplacement{{Before: `"Hello."`, After: `""`}}},
		{"last selected is not last array element", "[assistant]", `[{"role":"assistant","content":"Hello."},null]`, true, nil},
		{"history typed full strip", "[assistant]", `[{"role":"assistant","content":[{"type":"text","text":"Hello."}]},{"role":"user","content":"Continue."}]`, true, nil},
		{"last typed full strip", "[assistant]", `[{"role":"user","content":"Greet me."},{"role":"assistant","content":[{"type":"text","text":"Hello."}]}]`, true, nil},
		{"history typed partial strip", "[assistant]", `[{"role":"assistant","content":[{"type":"text","text":"Hello. tail"}]},{"role":"user","content":"Continue."}]`, false, []rawReplacement{{Before: `"Hello. tail"`, After: `" tail"`}}},
		{"last typed partial strip", "[assistant]", `[{"role":"user","content":"Greet me."},{"role":"assistant","content":[{"type":"text","text":"Hello. tail"}]}]`, false, []rawReplacement{{Before: `"Hello. tail"`, After: `" tail"`}}},
		{"user scalar full strip", "[user]", `[{"role":"user","content":"Hello."}]`, true, nil},
		{"history own scope disabled", "[tool]", `[{"role":"assistant","content":"Hello."},{"role":"user","content":"Continue."}]`, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "words:\n  strip: [Hello.]\nscope:\n  roles: "+tc.roles+"\n")
			body := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"untouched":"Hello. machine","messages":` + tc.messages + `}`)
			resp := interceptRPC(t, "claude", body)
			if tc.invalid {
				if !resp.Terminate || resp.StatusCode != http.StatusBadRequest || resp.Body != nil || gjson.GetBytes(resp.ResponseBody, "error.code").Str != "censorship_invalid_request" {
					t.Fatalf("want local invalid text, terminate=%t status=%d error=%s", resp.Terminate, resp.StatusCode, resp.ResponseBody)
				}
				return
			}
			if tc.replacements == nil {
				requireNoOpResponse(t, resp)
				return
			}
			want := replaceRawTokens(t, body, tc.replacements...)
			if resp.Terminate || !bytes.Equal(resp.Body, want) {
				t.Fatalf("body differs outside expected assistant text token, response=%#v", resp)
			}
		})
	}
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
