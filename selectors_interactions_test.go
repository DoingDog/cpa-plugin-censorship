package main

import (
	"bytes"
	"testing"

	"github.com/tidwall/gjson"
)

func TestInteractionsAnnotatedModelOutputRejectsRewrite(t *testing.T) {
	registerConfig(t, "words:\n  obfs: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"input":[{"type":"model_output","content":[{"type":"text","text":"SECRET","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com"}]}]}]}`)

	resp := interceptRPC(t, "interactions", body)
	if !resp.Terminate || resp.StatusCode != 400 || len(resp.Body) != 0 {
		t.Fatalf("response = %#v", resp)
	}
	if got := gjson.GetBytes(resp.ResponseBody, "error.code").String(); got != "censorship_invalid_request" {
		t.Fatalf("error code = %q, want %q", got, "censorship_invalid_request")
	}
	if got := gjson.GetBytes(resp.ResponseBody, "error.message").String(); got != "censorship cannot rewrite annotated text" {
		t.Fatalf("error message = %q, want %q", got, "censorship cannot rewrite annotated text")
	}
}

func TestInteractionsAnnotatedModelOutputStillBlocks(t *testing.T) {
	registerConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"input":[{"type":"model_output","content":[{"type":"text","text":"SECRET","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com"}]}]}]}`)

	resp := interceptRPC(t, "interactions", body)
	if !resp.Terminate || resp.StatusCode != 400 {
		t.Fatalf("response = %#v", resp)
	}
	if got := gjson.GetBytes(resp.ResponseBody, "error.term").String(); got != "SECRET" {
		t.Fatalf("error term = %q, want %q", got, "SECRET")
	}
	if got := gjson.GetBytes(resp.ResponseBody, "error.role").String(); got != "assistant" {
		t.Fatalf("error role = %q, want %q", got, "assistant")
	}
}

func TestInteractionsEmptyAnnotationsRemainRewritable(t *testing.T) {
	registerConfig(t, "words:\n  obfs: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"input":[{"type":"model_output","content":[{"type":"text","text":"SECRET","annotations":[]}]}]}`)

	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	if got := gjson.GetBytes(resp.Body, "input.0.content.0.text").String(); got != "S​ECRET" {
		t.Fatalf("text = %q, want %q", got, "S​ECRET")
	}
}

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

func TestInteractionsDirectTextContent(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	cases := []struct {
		name, body, want string
	}{
		{
			name: "direct object",
			body: `{"input":{"type":"text","text":"SECRET"}}`,
			want: `{"input":{"type":"text","text":""}}`,
		},
		{
			name: "direct array element",
			body: `{"input":[{"type":"text","text":"SECRET"}]}`,
			want: `{"input":[{"type":"text","text":""}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := interceptRPC(t, "interactions", []byte(tc.body))
			if resp.Terminate || string(resp.Body) != tc.want {
				t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, tc.want)
			}
		})
	}
}

func TestInteractionsDirectTextContentRespectsUserScope(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"input":{"type":"text","text":"SECRET"}}`)
	resp := interceptRPC(t, "interactions", body)
	got := resp.Body
	if len(got) == 0 {
		got = body
	}
	if resp.Terminate || !bytes.Equal(got, body) {
		t.Fatalf("response = %#v, body = %s, want unchanged %s", resp, got, body)
	}
}

func TestInteractionsDirectTextContentKeepsProtocolExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"input":[
		{"type":"text","text":"SECRET direct"},
		{"type":"text","text":"SECRET null camel","functionCall":null},
		{"type":"text","text":"SECRET null snake","function_call":null},
		{"type":"image","text":"SECRET image"},
		{"type":"text","text":"SECRET inline","inlineData":{"data":"SECRET"}},
		{"type":"text","text":"SECRET function","functionCall":{"name":"tool"}},
		{"type":"thought","text":"SECRET thought"},
		{"type":"function_call","content":"SECRET call"},
		{"type":"model_output","content":"SECRET output"},
		{"steps":[{"type":"text","text":"SECRET nested"}]}
	]}`)
	resp := interceptRPC(t, "interactions", body)
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET direct"`, After: `" direct"`},
		rawReplacement{Before: `"SECRET null camel"`, After: `" null camel"`},
		rawReplacement{Before: `"SECRET null snake"`, After: `" null snake"`},
	)
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
	}
}

func TestInteractionsDirectTextContentKeepsModelOutputAssistantOnly(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"input":[{"type":"text","text":"SECRET user"},{"type":"model_output","content":"SECRET assistant"}]}`)
	want := `{"input":[{"type":"text","text":"SECRET user"},{"type":"model_output","content":" assistant"}]}`
	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate || string(resp.Body) != want {
		t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
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
		{name: "function result", body: `{"input":[{"type":"function_result","result":"SECRET"}]}`, role: "tool"},
		{name: "mcp server tool result", body: `{"input":[{"type":"mcp_server_tool_result","result":"SECRET"}]}`, role: "tool"},
		{name: "code execution result", body: `{"input":[{"type":"code_execution_result","result":"SECRET"}]}`, role: "tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "interactions", tc.body, tc.role) })
	}
}

func TestInteractionsCallerAnnotationsRemainRewritable(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"input":{"type":"text","text":"SECRET user","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com"}]}}`)
	want := `{"input":{"type":"text","text":" user","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.com"}]}}`
	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate || string(resp.Body) != want {
		t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
	}
}

func TestInteractionsDocumentedToolResults(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [tool]\n")
	body := []byte(`{"input":[
	  {"type":"function_result","call_id":"SECRET id","name":"SECRET name","result":"SECRET function"},
	  {"type":"function_result","result":[{"type":"text","text":"SECRET typed"},{"type":"image","data":"SECRET image"},{"type":"unknown","text":"SECRET unknown"}]},
	  {"type":"function_result","content":"SECRET obsolete","result":{"text":"SECRET object"}},
	  {"type":"mcp_server_tool_result","call_id":"SECRET id","server_name":"SECRET server","name":"SECRET name","result":"SECRET mcp"},
	  {"type":"mcp_server_tool_result","result":[{"type":"text","text":"SECRET mcp typed"},{"type":"image","data":"SECRET image"}]},
	  {"type":"code_execution_result","call_id":"SECRET id","result":"SECRET code","is_error":false}
	]}`)
	want := `{"input":[
	  {"type":"function_result","call_id":"SECRET id","name":"SECRET name","result":" function"},
	  {"type":"function_result","result":[{"type":"text","text":" typed"},{"type":"image","data":"SECRET image"},{"type":"unknown","text":"SECRET unknown"}]},
	  {"type":"function_result","content":"SECRET obsolete","result":{"text":"SECRET object"}},
	  {"type":"mcp_server_tool_result","call_id":"SECRET id","server_name":"SECRET server","name":"SECRET name","result":" mcp"},
	  {"type":"mcp_server_tool_result","result":[{"type":"text","text":" mcp typed"},{"type":"image","data":"SECRET image"}]},
	  {"type":"code_execution_result","call_id":"SECRET id","result":" code","is_error":false}
	]}`
	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate || string(resp.Body) != want {
		t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
	}
}

func TestInteractionsSignedCodeExecutionResultRejectsRewrite(t *testing.T) {
	registerConfig(t, "words:\n  obfs: [SECRET]\nscope:\n  roles: [tool]\n")
	body := []byte(`{"input":[{"type":"code_execution_result","call_id":"call","result":"SECRET","signature":"signed"}]}`)
	resp := interceptRPC(t, "interactions", body)
	if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.message").String() != "censorship cannot rewrite signature-bound text" {
		t.Fatalf("response = %#v", resp)
	}

	t.Run("block reports tool", func(t *testing.T) {
		assertBlockedRole(t, "interactions", string(body), "tool")
	})
	t.Run("null signature permits rewrite", func(t *testing.T) {
		registerConfig(t, "words:\n  obfs: [SECRET]\nscope:\n  roles: [tool]\n")
		nullBody := []byte(`{"input":[{"type":"code_execution_result","call_id":"call","result":"SECRET","signature":null}]}`)
		resp := interceptRPC(t, "interactions", nullBody)
		if resp.Terminate || gjson.GetBytes(resp.Body, "input.0.result").String() != "S​ECRET" {
			t.Fatalf("response = %#v", resp)
		}
	})
}

func TestInteractionsFunctionToolsAndResponseSchemas(t *testing.T) {
	body := []byte(`{
	  "tools": [
	    {
	      "type": "function",
	      "name": "SECRET function name",
	      "description": "SECRET function description",
	      "parameters": {
	        "type": "object",
	        "description": "SECRET root schema",
	        "properties": {
	          "SECRET key": {
	            "type": "string",
	            "description": "SECRET property",
	            "enum": ["SECRET enum"],
	            "default": "SECRET default"
	          }
	        },
	        "required": ["SECRET key"]
	      }
	    },
	    {"type": "google_search", "description": "SECRET excluded"},
	    {"description": "SECRET missing type"}
	  ],
	  "response_format": [
	    {"type": "text", "mime_type": "application/json", "schema": {"type": "object", "description": "SECRET response", "properties": {"value": {"description": "SECRET nested response"}}}},
	    {"type": "image", "schema": {"description": "SECRET image schema"}},
	    {"schema": {"description": "SECRET missing type schema"}}
	  ]
	}`)
	want := `{
	  "tools": [
	    {
	      "type": "function",
	      "name": "SECRET function name",
	      "description": " function description",
	      "parameters": {
	        "type": "object",
	        "description": " root schema",
	        "properties": {
	          "SECRET key": {
	            "type": "string",
	            "description": " property",
	            "enum": ["SECRET enum"],
	            "default": "SECRET default"
	          }
	        },
	        "required": ["SECRET key"]
	      }
	    },
	    {"type": "google_search", "description": "SECRET excluded"},
	    {"description": "SECRET missing type"}
	  ],
	  "response_format": [
	    {"type": "text", "mime_type": "application/json", "schema": {"type": "object", "description": " response", "properties": {"value": {"description": " nested response"}}}},
	    {"type": "image", "schema": {"description": "SECRET image schema"}},
	    {"schema": {"description": "SECRET missing type schema"}}
	  ]
	}`
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [developer]\n")
	resp := interceptRPC(t, "interactions", body)
	if resp.Terminate || string(resp.Body) != want {
		t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
	}

	t.Run("single object response format", func(t *testing.T) {
		registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [developer]\n")
		body := []byte(`{"response_format":{"type":"text","schema":{"description":"SECRET response"}}}`)
		want := `{"response_format":{"type":"text","schema":{"description":" response"}}}`
		resp := interceptRPC(t, "interactions", body)
		if resp.Terminate || string(resp.Body) != want {
			t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
		}
	})
	t.Run("function tool block reports developer", func(t *testing.T) {
		assertBlockedRole(t, "interactions", `{"tools":[{"type":"function","description":"SECRET"}]}`, "developer")
	})
	t.Run("response schema block reports developer", func(t *testing.T) {
		assertBlockedRole(t, "interactions", `{"response_format":{"type":"text","schema":{"description":"SECRET"}}}`, "developer")
	})
	t.Run("user scope leaves definitions unchanged", func(t *testing.T) {
		registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
		resp := interceptRPC(t, "interactions", body)
		got := resp.Body
		if len(got) == 0 {
			got = body
		}
		if resp.Terminate || !bytes.Equal(got, body) {
			t.Fatalf("response = %#v, body = %s, want unchanged %s", resp, got, body)
		}
	})
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
		{name: "extra content Google thought signature null", part: `{"text":"accepted","extra_content":{"google":{"thought_signature":null}}}`, allowed: true},
		{name: "Gemini extra content Google thought signature non-null", part: `{"text":"accepted","extra_content":{"google":{"thought_signature":"sig"}}}`, allowed: true},
		{name: "Interactions extra content Google thought signature non-null", part: `{"text":"accepted","extra_content":{"google":{"thought_signature":"sig"}}}`, requireTextType: true, allowed: false},
		{name: "ordinary extra content", part: `{"text":"accepted","extra_content":{"google":{"note":"value"}}}`, requireTextType: true, allowed: true},
		{name: "escaped machine key", part: `{"text":"accepted","\u0066unctionCall":null}`, requireTextType: true, allowed: true},
	}
	machineKeys := []string{
		"functionCall", "function_call", "functionResponse", "function_response",
		"inlineData", "inline_data", "fileData", "file_data",
		"executableCode", "executable_code", "codeExecutionResult", "code_execution_result",
		"thoughtSignature", "thought_signature",
	}
	for _, key := range machineKeys {
		cases = append(cases,
			struct {
				name, part      string
				requireTextType bool
				allowed         bool
			}{
				name:            "machine key " + key + " is null",
				part:            `{"text":"accepted","` + key + `":null}`,
				requireTextType: true,
				allowed:         true,
			},
			struct {
				name, part      string
				requireTextType bool
				allowed         bool
			}{
				name:            "machine key " + key + " is non-null",
				part:            `{"text":"accepted","` + key + `":{}}`,
				requireTextType: true,
				allowed:         false,
			},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			part := gjson.Parse(tc.part)
			wantText := part.Get("text")
			gotText, gotAllowed, _ := scanTextPart(part, tc.requireTextType)
			if gotAllowed != tc.allowed {
				t.Fatalf("allowed = %t, want %t", gotAllowed, tc.allowed)
			}
			if gotText.Raw != wantText.Raw || gotText.Str != wantText.Str || gotText.Index != wantText.Index {
				t.Fatalf("text = {Raw:%q Str:%q Index:%d}, want {Raw:%q Str:%q Index:%d}", gotText.Raw, gotText.Str, gotText.Index, wantText.Raw, wantText.Str, wantText.Index)
			}
		})
	}
}

func TestInteractionsSystemInstructionObjectTextHonorsMachineExclusions(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system]\n")
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown image type",
			body: `{"system_instruction":{"type":"image","text":"SECRET"}}`,
			want: `{"system_instruction":{"type":"image","text":"SECRET"}}`,
		},
		{
			name: "text with inline data",
			body: `{"system_instruction":{"type":"text","text":"SECRET","inlineData":{"data":"SECRET"}}}`,
			want: `{"system_instruction":{"type":"text","text":"SECRET","inlineData":{"data":"SECRET"}}}`,
		},
		{
			name: "plain text object remains eligible",
			body: `{"system_instruction":{"type":"text","text":"SECRET"}}`,
			want: `{"system_instruction":{"type":"text","text":""}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := interceptRPC(t, "interactions", []byte(tc.body))
			if resp.Terminate {
				t.Fatalf("response = %#v, want non-terminating response", resp)
			}
			got := resp.Body
			if len(got) == 0 {
				got = []byte(tc.body)
			}
			if string(got) != tc.want {
				t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, tc.want)
			}
		})
	}
}
