package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestOpenAISelectorChangesOnlyContractedStringTokens(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, developer, user, assistant, tool]\n")
	base64Payload := strings.Repeat("SECRET", (20<<20)/len("SECRET")+1)[:20<<20]
	body := []byte(fmt.Sprintf(`{
  "SECRET-key":"unchanged",
  "messages":[
    {"role":"system","content":"SECRET system"},
    {"role":"developer","content":[{"type":"text","text":"SECRET developer"},{"type":"image_url","image_url":{"url":"SECRET"},"text":"SECRET excluded"}]},
    {"role":"user","content":[{"type":"text","text":"SECRET user"},{"type":"input_audio","data":"SECRET"}]},
    {"role":"assistant","content":"SECRET assistant","reasoning":"SECRET"},
    {"role":"tool","content":"SECRET tool"},
    {"role":"tool","content":[{"type":"text","text":"SECRET excluded tool array"}]},
    {"role":"unknown","content":"SECRET unknown"},
    {"content":"SECRET missing role"},
    {"role":"user","content":123}
  ],
  "tools":[{"type":"function","function":{"name":"SECRET","description":"SECRET","parameters":{"SECRET":"SECRET"}}}],
  "tool_calls":[{"function":{"name":"SECRET","arguments":"{\"text\":\"SECRET\"}"}}],
  "image":"data:image/png;base64,%s"
}`, base64Payload))
	resp := interceptRPC(t, "openai", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET system"`, After: `" system"`},
		rawReplacement{Before: `"SECRET developer"`, After: `" developer"`},
		rawReplacement{Before: `"SECRET user"`, After: `" user"`},
		rawReplacement{Before: `"SECRET assistant"`, After: `" assistant"`},
		rawReplacement{Before: `"SECRET tool"`, After: `" tool"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside contracted string tokens")
	}
}

func TestOpenAISelectorDefaultRolesExcludeAssistantAndTool(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	body := []byte(`{"messages":[{"role":"system","content":"SECRET s"},{"role":"developer","content":"SECRET d"},{"role":"user","content":"SECRET u"},{"role":"assistant","content":"SECRET a"},{"role":"tool","content":"SECRET t"}]}`)
	resp := interceptRPC(t, "openai", body)
	want := `{"messages":[{"role":"system","content":" s"},{"role":"developer","content":" d"},{"role":"user","content":" u"},{"role":"assistant","content":"SECRET a"},{"role":"tool","content":"SECRET t"}]}`
	if string(resp.Body) != want {
		t.Fatalf("body = %s, want %s", resp.Body, want)
	}
}

func TestOpenAISelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "system string", body: `{"messages":[{"role":"system","content":"SECRET"}]}`, role: "system"},
		{name: "developer string", body: `{"messages":[{"role":"developer","content":"SECRET"}]}`, role: "developer"},
		{name: "user string", body: `{"messages":[{"role":"user","content":"SECRET"}]}`, role: "user"},
		{name: "assistant string", body: `{"messages":[{"role":"assistant","content":"SECRET"}]}`, role: "assistant"},
		{name: "typed text", body: `{"messages":[{"role":"user","content":[{"type":"text","text":"SECRET"}]}]}`, role: "user"},
		{name: "tool string", body: `{"messages":[{"role":"tool","content":"SECRET"}]}`, role: "tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "openai", tc.body, tc.role) })
	}
}

func TestOpenAIResponsesSelectorRowsAndExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, developer, user, assistant, tool]\n")
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "top-level strings",
			body: `{"instructions":"SECRET system","input":"SECRET user","metadata":{"note":"SECRET"}}`,
			want: `{"instructions":" system","input":" user","metadata":{"note":"SECRET"}}`,
		},
		{
			name: "message items",
			body: `{"input":[
				{"type":"message","role":"system","content":"SECRET s"},
				{"type":"","role":"user","content":"SECRET empty type"},
				{"role":"developer","content":[{"type":"input_text","text":"SECRET d"},{"type":"","text":"SECRET e"},{"text":"SECRET m"}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET u"},{"type":"output_text","text":"SECRET wrong output"},{"type":"input_image","image_url":"SECRET"}]},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":"SECRET a"}]},
				{"type":"message","content":"SECRET no role"},
				{"type":"message","role":"tool","content":"SECRET bad role"},
				{"type":"message","role":"user","content":123},
				{"type":"function_call_output","role":"user","output":"SECRET result","content":"SECRET fake"},
				{"type":"custom_tool_call_output","role":"user","output":"SECRET custom"},
				{"type":"unknown","role":"user","content":"SECRET unknown"}
			],"tools":[{"name":"SECRET","description":"SECRET"}]}`,
			want: `{"input":[
				{"type":"message","role":"system","content":" s"},
				{"type":"","role":"user","content":" empty type"},
				{"role":"developer","content":[{"type":"input_text","text":" d"},{"type":"","text":" e"},{"text":" m"}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":" u"},{"type":"output_text","text":"SECRET wrong output"},{"type":"input_image","image_url":"SECRET"}]},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":" a"}]},
				{"type":"message","content":"SECRET no role"},
				{"type":"message","role":"tool","content":"SECRET bad role"},
				{"type":"message","role":"user","content":123},
				{"type":"function_call_output","role":"user","output":"SECRET result","content":"SECRET fake"},
				{"type":"custom_tool_call_output","role":"user","output":"SECRET custom"},
				{"type":"unknown","role":"user","content":"SECRET unknown"}
			],"tools":[{"name":"SECRET","description":"SECRET"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := interceptRPC(t, "openai-response", []byte(tc.body))
			if resp.Terminate || string(resp.Body) != tc.want {
				t.Fatalf("body = %s, want %s, terminate = %t", resp.Body, tc.want, resp.Terminate)
			}
		})
	}
}

func TestOpenAIResponsesSelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "instructions", body: `{"instructions":"SECRET"}`, role: "system"},
		{name: "input string", body: `{"input":"SECRET"}`, role: "user"},
		{name: "message string", body: `{"input":[{"type":"message","role":"developer","content":"SECRET"}]}`, role: "developer"},
		{name: "input text", body: `{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET"}]}]}`, role: "user"},
		{name: "output text", body: `{"input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"SECRET"}]}]}`, role: "assistant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "openai-response", tc.body, tc.role) })
	}
}
