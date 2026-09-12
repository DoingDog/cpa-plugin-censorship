package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
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
		"system_instruction":{"parts":[{"text":"SECRET snake"},{"text":"signed","thoughtSignature":"sig"}]},
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

func TestGeminiOmittedRoleFormsAlternateCanonicalRoles(t *testing.T) {
	forms := []struct {
		name, field string
	}{
		{name: "missing", field: ""},
		{name: "null", field: `"role":null,`},
		{name: "empty", field: `"role":"",`},
	}
	for _, form := range forms {
		t.Run(form.name+" first only", func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\n")
			body := []byte(`{"contents":[{` + form.field + `"parts":[{"text":"SECRET"}]}]}`)
			resp := interceptRPC(t, "gemini", body)
			if resp.Terminate {
				t.Fatalf("response = %#v", resp)
			}
			want := bytes.Replace(body, []byte(`"SECRET"`), []byte(`""`), 1)
			if !bytes.Equal(resp.Body, want) {
				t.Fatalf("body = %s, want %s", resp.Body, want)
			}
		})
		t.Run(form.name+" after explicit user", func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [assistant]\n")
			body := []byte(`{"contents":[{"role":"user","parts":[{"text":"prior"}]},{` + form.field + `"parts":[{"text":"SECRET assistant"}]},{"parts":[{"text":"SECRET user"}]}]}`)
			resp := interceptRPC(t, "gemini", body)
			if resp.Terminate {
				t.Fatalf("response = %#v", resp)
			}
			want := bytes.Replace(body, []byte(`"SECRET assistant"`), []byte(`" assistant"`), 1)
			if !bytes.Equal(resp.Body, want) {
				t.Fatalf("body = %s, want %s", resp.Body, want)
			}
		})
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
		{"text":"SECRET nested functionCall","functionCall":{"thought_signature":"sig"}},
		{"text":"SECRET nested functionResponse","functionResponse":{"thought_signature":"sig"}},
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

func TestGeminiDisabledRolesSkipContents(t *testing.T) {
	const content = `{"role":"user","parts":[{"text":"user"}]}`
	contents := strings.TrimSuffix(strings.Repeat(content+",", 1<<14), ",")
	body := []byte(`{"systemInstruction":{"parts":[{"text":"system"}]},"contents":[` + contents + `]}`)
	cases := []struct {
		name  string
		roles scopeSet
		text  string
		role  string
	}{
		{name: "empty", roles: scopeSet{}},
		{name: "system only", roles: scopeSet{"system": {}}, text: "system", role: "system"},
		{name: "tool only", roles: scopeSet{"tool": {}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := selectTextSpans(body, "gemini", tc.roles)
			if err != nil {
				t.Fatal(err)
			}
			if tc.text == "" {
				if len(spans) != 0 {
					t.Fatalf("spans = %#v, want none", spans)
				}
				return
			}
			if len(spans) != 1 || spans[0].Text != tc.text || spans[0].Role != tc.role {
				t.Fatalf("spans = %#v, want one %s %s span", spans, tc.role, tc.text)
			}
		})
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

func TestGeminiSignedVisibleTextUsesCanonicalRole(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{"camel system", `{"systemInstruction":{"parts":[{"text":"SECRET","thoughtSignature":"sig"}]}}`, "system"},
		{"snake assistant", `{"contents":[{"role":"model","parts":[{"text":"SECRET","thought_signature":"sig"}]}]}`, "assistant"},
		{"compatibility carrier", `{"contents":[{"role":"user","parts":[{"text":"SECRET","extra_content":{"google":{"thought_signature":"sig"}}}]}]}`, "user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertBlockedRole(t, "gemini", tc.body, tc.role)
		})
	}
}

func TestGeminiSignatureBoundVisibleTextRejectsRewrite(t *testing.T) {
	carriers := []string{
		`"thoughtSignature":"c2lnXHUwMDQx"`,
		`"thought_signature":"c2ln"`,
		`"extra_content":{"google":{"thought_signature":"c2ln"}}`,
	}
	for _, mode := range []string{"strip", "obfs"} {
		for _, carrier := range carriers {
			t.Run(mode+"/"+carrier, func(t *testing.T) {
				registerConfig(t, "words:\n  "+mode+": [SECRET]\nscope:\n  roles: [assistant]\n")
				body := []byte(`{"contents":[{"role":"model","parts":[{"text":"SECRET visible",` + carrier + `,"decoy":"SECRET"}]}]}`)
				resp := interceptRPC(t, "gemini", body)
				if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.code").String() != "censorship_invalid_request" || gjson.GetBytes(resp.ResponseBody, "error.message").String() != "censorship cannot rewrite signature-bound text" {
					t.Fatalf("response = %#v", resp)
				}
			})
		}
	}
}

func TestGeminiSignatureBoundVisibleTextNoMatchLeavesBodyUntouched(t *testing.T) {
	registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"contents":[{"role":"model","parts":[{"text":"visible","thoughtSignature":"c2lnXHUwMDQx","decoy":"SECRET"}]}]}`)
	resp := interceptRPC(t, "gemini", body)
	if resp.Terminate || len(resp.Body) != 0 || len(resp.ResponseBody) != 0 {
		t.Fatalf("response = %#v", resp)
	}
}

func TestGeminiNullMachineDiscriminatorsRemainSelectable(t *testing.T) {
	machineKeys := []string{
		"functionCall", "function_call", "functionResponse", "function_response",
		"inlineData", "inline_data", "fileData", "file_data",
		"executableCode", "executable_code", "codeExecutionResult", "code_execution_result",
	}
	for _, key := range machineKeys {
		t.Run(key+" null", func(t *testing.T) {
			text, allowed, signatureBound := scanTextPart(gjson.Parse(`{"text":"SECRET","`+key+`":null}`), false)
			if !allowed || signatureBound || text.Str != "SECRET" {
				t.Fatalf("scanTextPart() = %q, %t, %t", text.Str, allowed, signatureBound)
			}
		})
		t.Run(key+" object", func(t *testing.T) {
			_, allowed, _ := scanTextPart(gjson.Parse(`{"text":"SECRET","`+key+`":{}}`), false)
			if allowed {
				t.Fatal("non-null machine member remained selectable")
			}
		})
	}

	signatures := []string{
		`"thoughtSignature":null`,
		`"thought_signature":null`,
		`"extra_content":{"google":{"thought_signature":null}}`,
	}
	for _, signature := range signatures {
		t.Run(signature, func(t *testing.T) {
			text, allowed, signatureBound := scanTextPart(gjson.Parse(`{"text":"SECRET",`+signature+`}`), false)
			if !allowed || signatureBound || text.Str != "SECRET" {
				t.Fatalf("scanTextPart() = %q, %t, %t", text.Str, allowed, signatureBound)
			}
		})
	}
}

func TestGeminiSignatureBoundMachineDiscriminatorsRemainExcluded(t *testing.T) {
	signatures := []string{
		`"thoughtSignature":"sig"`,
		`"thought_signature":"sig"`,
		`"extra_content":{"google":{"thought_signature":"sig"}}`,
	}
	discriminators := []struct {
		name, member string
	}{
		{"thought", `"thought":true`},
		{"functionCall", `"functionCall":{}`},
		{"function_call", `"function_call":{}`},
		{"functionResponse", `"functionResponse":{}`},
		{"function_response", `"function_response":{}`},
		{"inlineData", `"inlineData":{}`},
		{"inline_data", `"inline_data":{}`},
		{"fileData", `"fileData":{}`},
		{"file_data", `"file_data":{}`},
		{"executableCode", `"executableCode":{}`},
		{"executable_code", `"executable_code":{}`},
		{"codeExecutionResult", `"codeExecutionResult":{}`},
		{"code_execution_result", `"code_execution_result":{}`},
	}
	for _, signature := range signatures {
		for _, discriminator := range discriminators {
			t.Run(signature+"/"+discriminator.name, func(t *testing.T) {
				_, allowed, _ := scanTextPart(gjson.Parse(`{"text":"SECRET",`+signature+`,`+discriminator.member+`}`), false)
				if allowed {
					t.Fatal("signature-bound machine part remained selectable")
				}
			})
		}
	}
}
