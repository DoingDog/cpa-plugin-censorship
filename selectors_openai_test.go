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
			body: []byte(`{"input":[{"type":"mcp_call","role":"user","id":"SECRET mcp id","name":"SECRET mcp name","arguments":"SECRET mcp arguments","server_label":"SECRET mcp server","output":"SECRET mcp output","error":"SECRET mcp error","url":"SECRET mcp url","status":"SECRET mcp status","files":["SECRET mcp file"],"screenshots":["SECRET mcp screenshot"]}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET mcp output"`, After: `" mcp output"`},
				{Before: `"SECRET mcp error"`, After: `" mcp error"`},
			},
		},
		{
			name: "program result",
			body: []byte(`{"input":[{"type":"program_result","role":"user","id":"SECRET program id","name":"SECRET program name","arguments":"SECRET program arguments","result":"SECRET program result","command":"SECRET program command","status":"SECRET program status","url":"SECRET program url","files":["SECRET program file"],"screenshots":["SECRET program screenshot"]},{"type":"unknown_result","id":"SECRET unknown id","output":"SECRET unknown output","result":"SECRET unknown result","error":"SECRET unknown error"}]}`),
			replacements: []rawReplacement{
				{Before: `"SECRET program result"`, After: `" program result"`},
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
