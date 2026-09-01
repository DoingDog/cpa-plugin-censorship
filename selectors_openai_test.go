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
