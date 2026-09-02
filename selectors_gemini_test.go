package main

import (
	"bytes"
	"testing"
)

func TestGeminiSelectorRowsAndMachineExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, user, assistant, tool]\n")
	body := []byte(`{
		"systemInstruction":{"parts":[
			{"text":"SECRET camel"},
			{"text":"SECRET functionCall","functionCall":{"name":"x"}},
			{"text":"SECRET functionResponse","functionResponse":{"response":{}}},
			{"text":"SECRET inlineData","inlineData":{"data":"SECRET"}},
			{"text":"SECRET inline_data","inline_data":{"data":"SECRET"}},
			{"text":"SECRET fileData","fileData":{"fileUri":"SECRET"}},
			{"text":"SECRET file_data","file_data":{"file_uri":"SECRET"}},
			{"text":"SECRET executableCode","executableCode":{"code":"SECRET"}},
			{"text":"SECRET codeExecutionResult","codeExecutionResult":{"output":"SECRET"}},
			{"text":"SECRET thought false","thought":false},
			{"text":"SECRET thought","thought":true}
		]},
		"system_instruction":{"parts":[{"text":"SECRET snake"},{"text":"SECRET signed","thoughtSignature":"sig"}]},
		"contents":[
			{"parts":[{"text":"SECRET inherited"}]},
			{"role":"user","parts":[{"text":"SECRET user"}]},
			{"role":"model","parts":[{"text":"SECRET model"}]},
			{"role":"assistant","parts":[{"text":"SECRET unknown role"}]}
		]
	}`)
	resp := interceptRPC(t, "gemini", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET camel"`, After: `" camel"`},
		rawReplacement{Before: `"SECRET thought false"`, After: `" thought false"`},
		rawReplacement{Before: `"SECRET snake"`, After: `" snake"`},
		rawReplacement{Before: `"SECRET inherited"`, After: `" inherited"`},
		rawReplacement{Before: `"SECRET user"`, After: `" user"`},
		rawReplacement{Before: `"SECRET model"`, After: `" model"`},
	)
	if !bytes.Equal(resp.Body, want) {
		t.Fatalf("body differs outside contracted Gemini string tokens")
	}
}

func TestGeminiMissingRolesAlternateCanonicalRoles(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"prior"}]},{"parts":[{"text":"SECRET assistant"}]},{"parts":[{"text":"SECRET user"}]}]}`)
	resp := interceptRPC(t, "gemini", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := `{"contents":[{"role":"user","parts":[{"text":"prior"}]},{"parts":[{"text":"SECRET assistant"}]},{"parts":[{"text":" user"}]}]}`
	if string(resp.Body) != want {
		t.Fatalf("body = %s, want %s", resp.Body, want)
	}
}

func TestGeminiInvalidRolesAdvanceCanonicalAlternation(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\n")
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"prior"}]},{"role":"assistant","parts":[{"text":"ignored"}]},{"parts":[{"text":"SECRET user"}]}]}`)
	resp := interceptRPC(t, "gemini", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := `{"contents":[{"role":"user","parts":[{"text":"prior"}]},{"role":"assistant","parts":[{"text":"ignored"}]},{"parts":[{"text":" user"}]}]}`
	if string(resp.Body) != want {
		t.Fatalf("body = %s, want %s", resp.Body, want)
	}
}

func TestGeminiSelectorExcludesSnakeCaseMachineParts(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"contents":[{"role":"model","parts":[
		{"text":"SECRET thought_signature","thought_signature":"sig"},
		{"text":"SECRET nested functionCall","functionCall":{"thought_signature":"sig"}},
		{"text":"SECRET nested functionResponse","functionResponse":{"thought_signature":"sig"}},
		{"text":"SECRET extra_content","extra_content":{"google":{"thought_signature":"sig"}}},
		{"text":"SECRET function_call","function_call":{"name":"tool"}},
		{"text":"SECRET function_response","function_response":{"response":{}}},
		{"text":"SECRET executable_code","executable_code":{"code":"SECRET"}},
		{"text":"SECRET code_execution_result","code_execution_result":{"output":"SECRET"}}
	]}]}`)
	resp := interceptRPC(t, "gemini", body)
	if resp.Terminate || len(resp.ResponseBody) != 0 || len(resp.Body) != 0 {
		t.Fatalf("response = %#v", resp)
	}
}

func TestGeminiSelectorCanonicalRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{name: "camel system", body: `{"systemInstruction":{"parts":[{"text":"SECRET"}]}}`, role: "system"},
		{name: "snake system", body: `{"system_instruction":{"parts":[{"text":"SECRET"}]}}`, role: "system"},
		{name: "missing role", body: `{"contents":[{"parts":[{"text":"SECRET"}]}]}`, role: "user"},
		{name: "user role", body: `{"contents":[{"role":"user","parts":[{"text":"SECRET"}]}]}`, role: "user"},
		{name: "model role", body: `{"contents":[{"role":"model","parts":[{"text":"SECRET"}]}]}`, role: "assistant"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "gemini", tc.body, tc.role) })
	}
}
