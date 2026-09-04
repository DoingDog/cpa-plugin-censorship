package main

import (
	"bytes"
	"testing"

	"github.com/tidwall/gjson"
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

func TestInteractionsContentMachinePartsAreExcluded(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	bodies := []string{
		`{"input":{"type":"user_input","content":{"type":"text","text":"SECRET","functionCall":{"name":"tool"}}}}`,
		`{"input":{"type":"user_input","content":[{"type":"text","text":"SECRET","inlineData":{"data":"SECRET"}}]}}`,
	}
	for _, body := range bodies {
		resp := interceptRPC(t, "interactions", []byte(body))
		if resp.Terminate || len(resp.Body) != 0 {
			t.Fatalf("response for %s = %#v", body, resp)
		}
	}
}

func TestInteractionsPartsExcludeSnakeCaseMachineFields(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"input":[{"type":"user_input","parts":[
		{"text":"SECRET thought_signature","thought_signature":"sig"},
		{"text":"SECRET nested functionCall","functionCall":{"thought_signature":"sig"}},
		{"text":"SECRET nested functionResponse","functionResponse":{"thought_signature":"sig"}},
		{"text":"SECRET extra_content","extra_content":{"google":{"thought_signature":"sig"}}},
		{"text":"SECRET function_call","function_call":{"name":"tool"}},
		{"text":"SECRET function_response","function_response":{"response":{}}},
		{"text":"SECRET executable_code","executable_code":{"code":"SECRET"}},
		{"text":"SECRET code_execution_result","code_execution_result":{"output":"SECRET"}}
	]}]}`)
	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate || len(resp.ResponseBody) != 0 || len(resp.Body) != 0 {
		t.Fatalf("response = %#v", resp)
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

func TestInteractionsCamelCaseSystemInstructionRows(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	cases := []struct {
		body, want string
	}{
		{body: `{"systemInstruction":"SECRET system"}`, want: `{"systemInstruction":" system"}`},
		{body: `{"systemInstruction":{"text":"SECRET object"}}`, want: `{"systemInstruction":{"text":" object"}}`},
		{body: `{"systemInstruction":{"parts":[{"type":"text","text":"SECRET part"}]}}`, want: `{"systemInstruction":{"parts":[{"type":"text","text":" part"}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			resp := interceptRPC(t, "interactions", []byte(tc.body))
			if resp.Terminate || string(resp.Body) != tc.want {
				t.Fatalf("body = %s, want %s", resp.Body, tc.want)
			}
		})
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

func TestScanTextPartPreservesProtocolRules(t *testing.T) {
	cases := []struct {
		name, part      string
		requireTextType bool
		allowed         bool
	}{
		{name: "reject non-object", part: `null`, allowed: false},
		{name: "Gemini missing type", part: `{"text":"accepted"}`, allowed: true},
		{name: "Interactions missing type", part: `{"text":"accepted"}`, requireTextType: true, allowed: true},
		{name: "Gemini empty type", part: `{"type":"","text":"accepted"}`, allowed: true},
		{name: "Interactions empty type", part: `{"type":"","text":"accepted"}`, requireTextType: true, allowed: true},
		{name: "Gemini text type", part: `{"type":"text","text":"accepted"}`, allowed: true},
		{name: "Interactions text type", part: `{"type":"text","text":"accepted"}`, requireTextType: true, allowed: true},
		{name: "Gemini invalid string type", part: `{"type":"image","text":"accepted"}`, allowed: true},
		{name: "Interactions invalid string type", part: `{"type":"image","text":"accepted"}`, requireTextType: true, allowed: false},
		{name: "Gemini non-string type", part: `{"type":1,"text":"accepted"}`, allowed: true},
		{name: "Interactions non-string type", part: `{"type":1,"text":"accepted"}`, requireTextType: true, allowed: false},
		{name: "Gemini thought true", part: `{"thought":true,"text":"accepted"}`, allowed: false},
		{name: "Interactions thought true", part: `{"thought":true,"text":"accepted"}`, requireTextType: true, allowed: false},
		{name: "Gemini thought false", part: `{"thought":false,"text":"accepted"}`, allowed: true},
		{name: "Interactions thought false", part: `{"thought":false,"text":"accepted"}`, requireTextType: true, allowed: true},
		{name: "camel nested functionCall thought signature", part: `{"text":"accepted","functionCall":{"thoughtSignature":null}}`, allowed: false},
		{name: "snake nested functionCall thought signature", part: `{"text":"accepted","functionCall":{"thought_signature":null}}`, allowed: false},
		{name: "camel nested functionResponse thought signature", part: `{"text":"accepted","functionResponse":{"thoughtSignature":null}}`, allowed: false},
		{name: "snake nested functionResponse thought signature", part: `{"text":"accepted","functionResponse":{"thought_signature":null}}`, allowed: false},
		{name: "extra content Google thought signature", part: `{"text":"accepted","extra_content":{"google":{"thought_signature":null}}}`, allowed: false},
		{name: "ordinary extra content", part: `{"text":"accepted","extra_content":{"google":{"note":"value"}}}`, requireTextType: true, allowed: true},
	}
	machineKeys := []string{
		"functionCall", "functionResponse", "function_call", "function_response",
		"inlineData", "inline_data", "fileData", "file_data",
		"executableCode", "executable_code", "codeExecutionResult", "code_execution_result",
		"thoughtSignature", "thought_signature",
	}
	for _, key := range machineKeys {
		cases = append(cases, struct {
			name, part      string
			requireTextType bool
			allowed         bool
		}{name: "machine key " + key + " is present when null", part: `{"text":"accepted","` + key + `":null}`, requireTextType: true})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			part := gjson.Parse(tc.part)
			wantText := part.Get("text")
			gotText, gotAllowed := scanTextPart(part, tc.requireTextType)
			if gotAllowed != tc.allowed {
				t.Fatalf("allowed = %t, want %t", gotAllowed, tc.allowed)
			}
			if gotText.Raw != wantText.Raw || gotText.Str != wantText.Str || gotText.Index != wantText.Index {
				t.Fatalf("text = {Raw:%q Str:%q Index:%d}, want {Raw:%q Str:%q Index:%d}", gotText.Raw, gotText.Str, gotText.Index, wantText.Raw, wantText.Str, wantText.Index)
			}
		})
	}
}
