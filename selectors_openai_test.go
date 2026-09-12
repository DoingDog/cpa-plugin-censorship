package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
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
  "tools":[{"type":"function","function":{"name":"SECRET tool name","description":"SECRET tool description","parameters":{"SECRET property":"SECRET schema value"}}}],
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
		rawReplacement{Before: `"SECRET tool description"`, After: `" tool description"`},
		rawReplacement{Before: `"SECRET excluded tool array"`, After: `" excluded tool array"`},
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
		{name: "function string", body: `{"messages":[{"role":"function","content":"SECRET"}]}`, role: "tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "openai", tc.body, tc.role) })
	}
}

func TestOpenAIChatToolTextArrayRoleGate(t *testing.T) {
	body := []byte(`{"messages":[{"role":"tool","tool_call_id":"SECRET tool id","name":"SECRET tool name","content":[{"type":"text","text":"SECRET tool text"},{"type":"image_url","image_url":{"url":"SECRET tool url"},"text":"SECRET image text"},{"type":"input_audio","input_audio":{"data":"SECRET audio"}}],"tool_calls":[{"id":"SECRET nested tool call","function":{"name":"SECRET nested tool name","arguments":"SECRET arguments"}}]}]}`)

	for _, tc := range []struct {
		name  string
		roles string
		want  []byte
	}{
		{
			name:  "tool enabled",
			roles: "tool",
			want: replaceRawTokens(t, body,
				rawReplacement{Before: `"SECRET tool text"`, After: `" tool text"`},
			),
		},
		{name: "tool disabled", roles: "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: ["+tc.roles+"]\n")
			resp := interceptRPC(t, "openai", body)
			if resp.Terminate {
				t.Fatalf("response = %#v", resp)
			}
			if tc.want == nil {
				if len(resp.Body) != 0 {
					t.Fatalf("body = %s, want empty body", resp.Body)
				}
				return
			}
			if !bytes.Equal(resp.Body, tc.want) {
				t.Fatalf("body = %s, want %s", resp.Body, tc.want)
			}
		})
	}
}

func TestOpenAIChatRefusalRoleGate(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","id":"SECRET assistant id","refusal":"SECRET top-level refusal","content":[{"type":"refusal","refusal":"SECRET part refusal","id":"SECRET refusal part id"},{"type":"image_url","image_url":{"url":"SECRET image url"},"text":"SECRET image text"}],"tool_calls":[{"id":"SECRET tool call","function":{"name":"SECRET tool name","arguments":"SECRET arguments"}}]}]}`)

	for _, tc := range []struct {
		name  string
		roles string
		want  []byte
	}{
		{
			name:  "assistant enabled",
			roles: "assistant",
			want: replaceRawTokens(t, body,
				rawReplacement{Before: `"SECRET top-level refusal"`, After: `" top-level refusal"`},
				rawReplacement{Before: `"SECRET part refusal"`, After: `" part refusal"`},
			),
		},
		{name: "assistant disabled", roles: "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: ["+tc.roles+"]\n")
			resp := interceptRPC(t, "openai", body)
			if resp.Terminate {
				t.Fatalf("response = %#v", resp)
			}
			if tc.want == nil {
				if len(resp.Body) != 0 {
					t.Fatalf("body = %s, want empty body", resp.Body)
				}
				return
			}
			if !bytes.Equal(resp.Body, tc.want) {
				t.Fatalf("body = %s, want %s", resp.Body, tc.want)
			}
		})
	}
}

func TestOpenAIChatFunctionRoleGate(t *testing.T) {
	body := []byte(`{"messages":[{"role":"function","name":"SECRET function name","tool_call_id":"SECRET function call id","content":"SECRET function result","tool_calls":[{"id":"SECRET nested tool call","function":{"name":"SECRET nested tool name","arguments":"SECRET arguments"}}]}]}`)

	for _, tc := range []struct {
		name  string
		roles string
		want  []byte
	}{
		{
			name:  "tool enabled",
			roles: "tool",
			want: replaceRawTokens(t, body,
				rawReplacement{Before: `"SECRET function result"`, After: `" function result"`},
			),
		},
		{name: "tool disabled", roles: "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: ["+tc.roles+"]\n")
			resp := interceptRPC(t, "openai", body)
			if resp.Terminate {
				t.Fatalf("response = %#v", resp)
			}
			if tc.want == nil {
				if len(resp.Body) != 0 {
					t.Fatalf("body = %s, want empty body", resp.Body)
				}
				return
			}
			if !bytes.Equal(resp.Body, tc.want) {
				t.Fatalf("body = %s, want %s", resp.Body, tc.want)
			}
		})
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
				{"type":"function_call_output","role":"user","output":" result","content":"SECRET fake"},
				{"type":"custom_tool_call_output","role":"user","output":" custom"},
				{"type":"unknown","role":"user","content":"SECRET unknown"}
			],"tools":[{"name":"SECRET","description":"SECRET"}]}`,
			want: `{"input":[
				{"type":"message","role":"system","content":" s"},
				{"type":"","role":"user","content":" empty type"},
				{"role":"developer","content":[{"type":"input_text","text":" d"},{"type":"","text":" e"},{"text":" m"}]},
				{"type":"message","role":"user","content":[{"type":"input_text","text":" u"},{"type":"output_text","text":" wrong output"},{"type":"input_image","image_url":"SECRET"}]},
				{"type":"message","role":"assistant","content":[{"type":"output_text","text":" a"}]},
				{"type":"message","content":"SECRET no role"},
				{"type":"message","role":"tool","content":"SECRET bad role"},
				{"type":"message","role":"user","content":123},
				{"type":"function_call_output","role":"user","output":" result","content":"SECRET fake"},
				{"type":"custom_tool_call_output","role":"user","output":" custom"},
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

func TestOpenAIResponsesOutputRoleGate(t *testing.T) {
	cases := []struct {
		name         string
		body         []byte
		replacements []rawReplacement
	}{
		{
			name: "function call output",
			body: []byte(`{"input":[{"type":"function_call_output","role":"user","id":"SECRET function id","call_id":"SECRET function call id","name":"SECRET function name","arguments":"SECRET function arguments","output":"SECRET function string"},{"type":"function_call_output","id":"SECRET function structured id","output":[{"type":"input_text","text":"SECRET function structured text"},{"type":"input_image","image_url":"SECRET function image url"},{"type":"input_file","file_data":"SECRET function file data","filename":"SECRET function filename"},{"type":"unknown","text":"SECRET function unknown text"}]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET function string"`, After: `" function string"`},
				{Before: `"SECRET function structured text"`, After: `" function structured text"`},
			},
		},
		{
			name: "custom tool call output",
			body: []byte(`{"input":[{"type":"custom_tool_call_output","role":"user","id":"SECRET custom id","call_id":"SECRET custom call id","name":"SECRET custom name","arguments":"SECRET custom arguments","output":"SECRET custom string"},{"type":"custom_tool_call_output","id":"SECRET custom structured id","output":[{"type":"input_text","text":"SECRET custom structured text"},{"type":"input_image","image_url":"SECRET custom image url"},{"type":"input_file","file_data":"SECRET custom file data","filename":"SECRET custom filename"},{"type":"unknown","text":"SECRET custom unknown text"}]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET custom string"`, After: `" custom string"`},
				{Before: `"SECRET custom structured text"`, After: `" custom structured text"`},
			},
		},
		{
			name: "local shell call output",
			body: []byte(`{"input":[{"type":"local_shell_call_output","role":"user","id":"SECRET local shell id","call_id":"SECRET local shell call id","output":"SECRET local shell output","command":"SECRET local shell command","status":"SECRET local shell status","files":["SECRET local shell file"],"screenshots":["SECRET local shell screenshot"]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET local shell output"`, After: `" local shell output"`},
			},
		},
		{
			name: "shell call output",
			body: []byte(`{"input":[{"type":"shell_call_output","role":"user","id":"SECRET shell id","call_id":"SECRET shell call id","command":"SECRET shell command","status":"SECRET shell status","output":[{"stdout":"SECRET shell stdout","stderr":"SECRET shell stderr","command":"SECRET nested shell command","status":"SECRET nested shell status","url":"SECRET nested shell url","files":["SECRET nested shell file"],"screenshots":["SECRET nested shell screenshot"],"result":{"message":"SECRET arbitrary shell text"}}]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET shell stdout"`, After: `" shell stdout"`},
				{Before: `"SECRET shell stderr"`, After: `" shell stderr"`},
			},
		},
		{
			name: "apply patch call output",
			body: []byte(`{"input":[{"type":"apply_patch_call_output","role":"user","id":"SECRET patch id","call_id":"SECRET patch call id","output":"SECRET patch output","patch":"SECRET patch text","command":"SECRET patch command","status":"SECRET patch status","files":["SECRET patch file"]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET patch output"`, After: `" patch output"`},
			},
		},
		{
			name: "mcp call",
			body: []byte(`{"input":[{"type":"mcp_call","id":"SECRET mcp id","name":"SECRET mcp name","arguments":"SECRET mcp arguments","server_label":"SECRET mcp server","output":"SECRET mcp output","error":{"type":"mcp_protocol_error","message":"SECRET protocol message","code":"SECRET protocol code"}},{"type":"mcp_call","output":null,"error":{"type":"http_error","message":"SECRET HTTP message","status_code":500}},{"type":"mcp_call","error":{"type":"future_error","message":"SECRET future message"}}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET mcp output"`, After: `" mcp output"`},
				{Before: `"SECRET protocol message"`, After: `" protocol message"`},
				{Before: `"SECRET HTTP message"`, After: `" HTTP message"`},
			},
		},
		{
			name: "mcp list tools error",
			body: []byte(`{"input":[{"type":"mcp_list_tools","id":"SECRET list id","server_label":"SECRET server","error":"SECRET list error","tools":[{"name":"SECRET tool","description":"SECRET description","input_schema":{"note":"SECRET schema"}}]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET list error"`, After: `" list error"`},
				{Before: `"SECRET description"`, After: `" description"`},
			},
		},
		{
			name: "program output",
			body: []byte(`{"input":[{"type":"program_output","id":"SECRET program id","result":"SECRET program result","status":"SECRET program status"},{"type":"program_result","result":"SECRET legacy result"},{"type":"unknown_result","result":"SECRET unknown result"}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET program result"`, After: `" program result"`},
			},
		},
		{
			name: "file search results",
			body: []byte(`{"input":[{"type":"file_search_call","id":"SECRET file search id","queries":["SECRET query"],"results":[{"file_id":"SECRET file id","filename":"SECRET filename","score":0.75,"text":"SECRET first result","attributes":{"note":"SECRET attribute"}},{"text":"SECRET second result"},{"text":123}]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET first result"`, After: `" first result"`},
				{Before: `"SECRET second result"`, After: `" second result"`},
			},
		},
		{
			name: "code interpreter logs",
			body: []byte(`{"input":[{"type":"code_interpreter_call","id":"SECRET code id","code":"SECRET code","container_id":"SECRET container","outputs":[{"type":"logs","logs":"SECRET logs"},{"type":"image","url":"SECRET image URL","logs":"SECRET image logs"},{"type":"future","logs":"SECRET future logs"}]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET logs"`, After: `" logs"`},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/tool enabled", func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [tool]\n")
			resp := interceptRPC(t, "openai-response", tc.body)
			want := replaceRawTokens(t, tc.body, tc.replacements...)
			if resp.Terminate || !bytes.Equal(resp.Body, want) {
				t.Fatalf("body = %s, want %s, terminate = %t", resp.Body, want, resp.Terminate)
			}
		})

		t.Run(tc.name+"/tool disabled", func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [user]\n")
			resp := interceptRPC(t, "openai-response", tc.body)
			if resp.Terminate || len(resp.Body) != 0 {
				t.Fatalf("body = %s, want empty body, terminate = %t", resp.Body, resp.Terminate)
			}
		})
	}
}

func TestOpenAIResponsesAssistantOutputShapesUseAssistantScope(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"input":[
		{"type":"message","role":"user","content":[{"type":"output_text","text":"SECRET user output"}]},
		{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"SECRET refusal"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET user input"}]}
	]}`)
	resp := interceptRPC(t, "openai-response", body)
	if resp.Terminate {
		t.Fatalf("response = %#v", resp)
	}
	want := `{"input":[
		{"type":"message","role":"user","content":[{"type":"output_text","text":" user output"}]},
		{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":" refusal"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"SECRET user input"}]}
	]}`
	if string(resp.Body) != want {
		t.Fatalf("body = %s, want %s", resp.Body, want)
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

func TestOpenAIChatModelVisibleDefinitionsUseDeclaredRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{
			name: "prediction string",
			body: `{"prediction":{"type":"content","content":"SECRET"}}`,
			role: "assistant",
		},
		{
			name: "prediction typed text",
			body: `{"prediction":{"type":"content","content":[{"type":"text","text":"SECRET"}]}}`,
			role: "assistant",
		},
		{
			name: "legacy function",
			body: `{"functions":[{"name":"safe","description":"SECRET","parameters":{"type":"object","description":"SECRET"}}]}`,
			role: "developer",
		},
		{
			name: "function tool",
			body: `{"tools":[{"type":"function","function":{"name":"safe","description":"SECRET","parameters":{"type":"object","description":"SECRET"},"output_schema":{"type":"object","description":"SECRET"}}}]}`,
			role: "developer",
		},
		{
			name: "custom tool",
			body: `{"tools":[{"type":"custom","custom":{"name":"safe","description":"SECRET","format":{"type":"grammar","definition":"SECRET"}}}]}`,
			role: "developer",
		},
		{
			name: "response format",
			body: `{"response_format":{"type":"json_schema","json_schema":{"name":"safe","description":"SECRET","schema":{"type":"object","description":"SECRET"}}}}`,
			role: "developer",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "openai", tc.body, tc.role) })
	}
}

func TestOpenAIChatDefinitionRewritePhasesPreserveMachineFields(t *testing.T) {
	for _, tc := range []struct {
		name, config, replacement string
	}{
		{name: "block", config: "mode: block\nwords: [SECRET]\nscope:\n  roles: [developer]\n"},
		{name: "strip", config: "mode: strip\nwords: [SECRET]\nscope:\n  roles: [developer]\n", replacement: " description"},
		{name: "obfs", config: "mode: obfs\nwords: [SECRET]\nscope:\n  roles: [developer]\n", replacement: "S​ECRET description"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, tc.config)
			body := []byte(`{"tools":[{"type":"function","function":{"name":"SECRET name","description":"SECRET description","parameters":{"type":"object","properties":{"SECRET property":{"type":"string"}},"x-grammar":"SECRET grammar"}}}],"tool_calls":[{"function":{"arguments":"SECRET arguments"}}]}`)
			resp := interceptRPC(t, "openai", body)
			if tc.name == "block" {
				if !resp.Terminate || gjson.GetBytes(resp.ResponseBody, "error.role").String() != "developer" {
					t.Fatalf("response = %#v", resp)
				}
				return
			}
			want := replaceRawTokens(t, body, rawReplacement{Before: `"SECRET description"`, After: `"` + tc.replacement + `"`})
			if resp.Terminate || !bytes.Equal(resp.Body, want) {
				t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
			}
		})
	}
}

func TestOpenAIChatSkipsUnknownPredictionType(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [assistant]\n")
	body := []byte(`{"prediction":{"type":"unknown","content":"SECRET"}}`)
	resp := interceptRPC(t, "openai", body)
	if resp.Terminate || len(resp.Body) != 0 {
		t.Fatalf("response = %#v", resp)
	}
}

func TestOpenAIResponsesModelVisibleDefinitionsUseDeclaredRoles(t *testing.T) {
	cases := []struct {
		name, body, role string
	}{
		{
			name: "prompt scalar variable",
			body: `{"prompt":{"id":"pmpt_safe","variables":{"customer":"SECRET"}}}`,
			role: "user",
		},
		{
			name: "prompt typed variable",
			body: `{"prompt":{"id":"pmpt_safe","variables":{"customer":{"type":"input_text","text":"SECRET"}}}}`,
			role: "user",
		},
		{
			name: "text format",
			body: `{"text":{"format":{"type":"json_schema","name":"safe","description":"SECRET","schema":{"type":"object","description":"SECRET"}}}}`,
			role: "developer",
		},
		{
			name: "top level function",
			body: `{"tools":[{"type":"function","name":"safe","description":"SECRET","parameters":{"type":"object","description":"SECRET"},"output_schema":{"type":"object","description":"SECRET"}}]}`,
			role: "developer",
		},
		{
			name: "top level custom",
			body: `{"tools":[{"type":"custom","name":"safe","description":"SECRET","format":{"type":"grammar","definition":"SECRET"}}]}`,
			role: "developer",
		},
		{
			name: "namespace child",
			body: `{"tools":[{"type":"namespace","name":"safe","description":"SECRET","tools":[{"type":"function","name":"child","description":"SECRET"}]}]}`,
			role: "developer",
		},
		{
			name: "tool search",
			body: `{"tools":[{"type":"tool_search","description":"SECRET","parameters":{"type":"object","description":"SECRET"}}]}`,
			role: "developer",
		},
		{
			name: "mcp",
			body: `{"tools":[{"type":"mcp","server_label":"safe","server_description":"SECRET","server_url":"https://SECRET.invalid"}]}`,
			role: "developer",
		},
		{
			name: "additional tools",
			body: `{"input":[{"type":"additional_tools","role":"assistant","tools":[{"type":"function","name":"safe","description":"SECRET"}]}]}`,
			role: "assistant",
		},
		{
			name: "loaded tool search",
			body: `{"input":[{"type":"tool_search_output","tools":[{"type":"function","name":"safe","description":"SECRET"}]}]}`,
			role: "tool",
		},
		{
			name: "mcp list tools",
			body: `{"input":[{"type":"mcp_list_tools","tools":[{"name":"safe","description":"SECRET","input_schema":{"type":"object","description":"SECRET"}}]}]}`,
			role: "tool",
		},
		{
			name: "local skill",
			body: `{"tools":[{"type":"shell","environment":{"type":"local","skills":[{"name":"safe","description":"SECRET","path":"/SECRET/path"}]}}]}`,
			role: "user",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertBlockedRole(t, "openai-response", tc.body, tc.role) })
	}
}

func TestOpenAIResponsesTopLevelLocalSkillUsesUserScope(t *testing.T) {
	registerConfig(t, "mode: block\nwords: [SECRET]\nscope:\n  roles: [user]\n")
	body := []byte(`{"tools":[{"type":"shell","environment":{"type":"local","skills":[{"name":"safe","description":"SECRET","path":"/SECRET/path"}]}}]}`)
	resp := interceptRPC(t, "openai-response", body)
	if !resp.Terminate || resp.StatusCode != 400 || gjson.GetBytes(resp.ResponseBody, "error.role").String() != "user" {
		t.Fatalf("response = %#v body = %s", resp, resp.ResponseBody)
	}
}

func TestOpenAIResponsesAdditionalToolsSkipsUnsupportedRoles(t *testing.T) {
	for _, tc := range []struct {
		name, role string
	}{
		{name: "missing"},
		{name: "null", role: `"role":null,`},
		{name: "number", role: `"role":1,`},
		{name: "unknown", role: `"role":"unknown",`},
		{name: "critic", role: `"role":"critic",`},
		{name: "discriminator", role: `"role":"discriminator",`},
		{name: "future", role: `"role":"future",`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [system, developer, user, assistant, tool]\n")
			body := []byte(`{"input":[{"type":"additional_tools",` + tc.role + `"tools":[{"type":"function","name":"SECRET","description":"SECRET","parameters":{"type":"object","description":"SECRET"}}]}]}`)
			resp := interceptRPC(t, "openai-response", body)
			if resp.Terminate || len(resp.Body) != 0 {
				t.Fatalf("response = %#v", resp)
			}
		})
	}
}

func TestOpenAIResponsesDefinitionRewritePreservesMachineFields(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [tool]\n")
	body := []byte(`{"input":[{"type":"tool_search_output","tools":[{"type":"function","name":"SECRET name","description":"SECRET description","parameters":{"type":"object","properties":{"SECRET property":{"type":"string","enum":["SECRET enum"],"default":"SECRET default","examples":["SECRET example"]}}}}],"arguments":"SECRET arguments","input":"SECRET input"}],"reasoning":{"summary":"SECRET reasoning","encrypted_content":"SECRET encrypted"}}`)
	resp := interceptRPC(t, "openai-response", body)
	want := replaceRawTokens(t, body, rawReplacement{Before: `"SECRET description"`, After: `" description"`})
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
	}
}

func TestOpenAIChatSchemaDescriptionsChangeWithoutMachineFields(t *testing.T) {
	registerConfig(t, "mode: strip\nwords: [SECRET]\nscope:\n  roles: [developer]\n")
	body := []byte(`{"functions":[{"name":"SECRET name","description":"SECRET root","parameters":{"type":"object","description":"SECRET object","properties":{"SECRET property":{"type":"string","description":"SECRET property description","enum":["SECRET enum"],"const":"SECRET const","default":"SECRET default","examples":["SECRET example"],"x-extra":{"description":"SECRET extension"}}},"items":{"type":"string","description":"SECRET items"},"allOf":[{"description":"SECRET all of"}]}}],"tool_calls":[{"function":{"arguments":"SECRET arguments"}}]}`)
	resp := interceptRPC(t, "openai", body)
	want := replaceRawTokens(t, body,
		rawReplacement{Before: `"SECRET root"`, After: `" root"`},
		rawReplacement{Before: `"SECRET object"`, After: `" object"`},
		rawReplacement{Before: `"SECRET property description"`, After: `" property description"`},
		rawReplacement{Before: `"SECRET items"`, After: `" items"`},
		rawReplacement{Before: `"SECRET all of"`, After: `" all of"`},
	)
	if resp.Terminate || !bytes.Equal(resp.Body, want) {
		t.Fatalf("response = %#v, body = %s, want %s", resp, resp.Body, want)
	}
}
