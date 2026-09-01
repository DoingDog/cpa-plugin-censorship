package main

import (
	"bytes"
	"testing"
)

func TestInteractionsSelectorRowsRoleInheritanceAndExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, user, assistant, tool]\n")
	body := []byte(`{
		"system_instruction":{"text":"SECRET sys text","parts":[{"type":"text","text":"SECRET sys part"},{"text":"SECRET sys missing"},{"type":"","text":"SECRET sys empty"},{"type":"text","text":"SECRET sys machine","inlineData":{"data":"SECRET"}},{"type":"image","text":"SECRET image"}]},
		"input":[
			"SECRET direct",
			{"type":"user_input","content":"SECRET content","parts":[{"text":"SECRET part"},{"text":"SECRET machine","inlineData":{"data":"SECRET"}}]},
			{"role":"model","content":[{"type":"text","text":"SECRET inherited assistant"}]},
			{"type":"model_output","role":"user","content":{"type":"text","text":"SECRET forced assistant"}},
			{"role":"user","steps":[{"content":"SECRET nested user"},{"role":"assistant","parts":[{"type":"","text":"SECRET nested assistant"}]}]},
			{"type":"thought","content":"SECRET thought"},
			{"type":"function_call","content":"SECRET call"},
			{"type":"function_result","content":"SECRET result"},
			{"type":"unknown","content":"SECRET unknown"},
			{"role":"tool","content":"SECRET bad role"},
			{"type":"user_input","content":123,"parts":[{"type":"text","text":123}]},
			{"steps":["SECRET string step"]}
		]
	}`)
	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET sys text"`, After: `" sys text"`},
		rawReplacement{Before: `"SECRET sys part"`, After: `" sys part"`},
		rawReplacement{Before: `"SECRET sys missing"`, After: `" sys missing"`},
		rawReplacement{Before: `"SECRET sys empty"`, After: `" sys empty"`},
		rawReplacement{Before: `"SECRET direct"`, After: `" direct"`},
		rawReplacement{Before: `"SECRET content"`, After: `" content"`},
		rawReplacement{Before: `"SECRET part"`, After: `" part"`},
		rawReplacement{Before: `"SECRET inherited assistant"`, After: `" inherited assistant"`},
		rawReplacement{Before: `"SECRET forced assistant"`, After: `" forced assistant"`},
		rawReplacement{Before: `"SECRET nested user"`, After: `" nested user"`},
		rawReplacement{Before: `"SECRET nested assistant"`, After: `" nested assistant"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside contracted Interactions string tokens")
	}
}

func TestInteractionsTopLevelStringRows(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, user]\n")
	cases := []struct {
		body, want string
	}{
		{body: `{"system_instruction":"SECRET system"}`, want: `{"system_instruction":" system"}`},
		{body: `{"input":"SECRET user"}`, want: `{"input":" user"}`},
	}
	for _, tc := range cases {
		resp := interceptRPC(t, "interactions", []byte(tc.body))
		if string(resp.Body) != tc.want {
			t.Fatalf("body = %s, want %s", resp.Body, tc.want)
		}
	}
}

func TestInteractionsSelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "system string", body: `{"system_instruction":"SECRET"}`, role: "system"},
		{name: "system object", body: `{"system_instruction":{"text":"SECRET"}}`, role: "system"},
		{name: "system part", body: `{"system_instruction":{"parts":[{"type":"","text":"SECRET"}]}}`, role: "system"},
		{name: "input string", body: `{"input":"SECRET"}`, role: "user"},
		{name: "input array string", body: `{"input":["SECRET"]}`, role: "user"},
		{name: "item content", body: `{"input":[{"type":"user_input","content":"SECRET"}]}`, role: "user"},
		{name: "content array", body: `{"input":[{"role":"assistant","content":[{"type":"text","text":"SECRET"}]}]}`, role: "assistant"},
		{name: "content object model output", body: `{"input":[{"type":"model_output","content":{"type":"text","text":"SECRET"}}]}`, role: "assistant"},
		{name: "parts", body: `{"input":[{"role":"user","parts":[{"text":"SECRET"}]}]}`, role: "user"},
		{name: "nested steps", body: `{"input":[{"role":"assistant","steps":[{"content":"SECRET"}]}]}`, role: "assistant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "interactions", tc.body, tc.role) })
	}
}
