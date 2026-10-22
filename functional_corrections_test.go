package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestFunctionalSelectorEntryPointOrdering(t *testing.T) {
	for _, tc := range []struct{ source, body, firstRole, lastRole string }{
		{"openai", `{"tools":[{"type":"function","function":{"description":"SECRET tail"}}],"messages":[{"role":"user","content":"SECRET tail"}]}`, "developer", "user"},
		{"openai-response", `{"input":"SECRET tail","instructions":"SECRET tail"}`, "user", "system"},
		{"claude", `{"messages":[{"role":"user","content":"SECRET tail"}],"system":"SECRET tail"}`, "user", "system"},
		{"gemini", `{"contents":[{"role":"user","parts":[{"text":"SECRET tail"}]}],"systemInstruction":{"parts":[{"text":"SECRET tail"}]}}`, "user", "system"},
		{"interactions", `{"input":"SECRET tail","system_instruction":"SECRET tail"}`, "user", "system"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			body := []byte(tc.body)
			roles := scopeSet{"system": {}, "developer": {}, "user": {}}
			spans, err := selectTextSpans(body, tc.source, roles)
			if err != nil || len(spans) != 2 {
				t.Fatalf("spans=%#v err=%v; want two sorted spans", spans, err)
			}
			starts := []int{bytes.Index(body, []byte(`"SECRET tail"`)), bytes.LastIndex(body, []byte(`"SECRET tail"`))}
			for i, role := range []string{tc.firstRole, tc.lastRole} {
				if spans[i].RawStart != starts[i] || spans[i].RawEnd != starts[i]+len(`"SECRET tail"`) || spans[i].Text != "SECRET tail" || spans[i].Role != role {
					t.Fatalf("span[%d]=%#v; want token at %d with role %s", i, spans[i], starts[i], role)
				}
			}
			blocked, err := transformRequest(body, tc.source, mustConfig(t, "words:\n  block: [SECRET]\nscope:\n  roles: [system, developer, user]\n"))
			if err != nil || blocked.Invalid || blocked.Body != nil || !reflect.DeepEqual(blocked.Blocked, &blockMatch{Term: "SECRET", Role: tc.firstRole}) {
				t.Fatalf("blocked=%#v err=%v; want earliest outer token", blocked, err)
			}
			result, err := transformRequest(body, tc.source, mustConfig(t, "words:\n  obfs: [SECRET]\nscope:\n  roles: [system, developer, user]\n"))
			want := []byte(strings.ReplaceAll(tc.body, `"SECRET tail"`, `"S​ECRET tail"`))
			if err != nil || result.Invalid || result.Blocked != nil || !bytes.Equal(result.Body, want) || string(body) != tc.body {
				t.Fatalf("body=%s want=%s result=%#v err=%v", result.Body, want, result, err)
			}
		})
	}
}

type functionalFixture struct {
	name, format, role, fragment string
	carrier                      bool
}

func functionalFixtures() []functionalFixture {
	fixtures := []functionalFixture{
		{"Chat_flat_custom", "openai", "developer", `"messages":[{"role":"user","content":"safe"}],"tools":[{"type":"custom","name":"SECRET name","description":%s,"format":{"type":"grammar","syntax":"lark","definition":"start: \"SECRET\""},"id":"SECRET id"}]`, false},
		{"Chat_schema_root", "openai", "developer", `"messages":[{"role":"user","content":"safe"}],"tools":[{"type":"function","function":{"name":"SECRET name","parametersJsonSchema":{"description":%s,"properties":{"SECRET key":{"enum":["SECRET enum"],"default":"SECRET default"}}}}}]`, false},
		{"Chat_schema_property", "openai", "developer", `"messages":[],"tools":[{"type":"function","function":{"name":"SECRET name","parametersJsonSchema":{"properties":{"SECRET key":{"description":%s,"const":"SECRET const"}}}}}]`, false},
		{"Responses_format_root", "openai-response", "developer", `"input":"safe","text":{"format":{"type":"json_schema","name":"SECRET name","json_schema":{"schema":{"description":%s,"enum":["SECRET enum"]}}}}`, false},
		{"Responses_format_property", "openai-response", "developer", `"input":"safe","text":{"format":{"type":"json_schema","json_schema":{"schema":{"properties":{"SECRET key":{"description":%s,"default":"SECRET default"}}}}}}`, false},
		{"Claude_MCP_assistant_scalar", "claude", "tool", `"messages":[{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"SECRET id","is_error":false,"content":%s,"cache_control":{"type":"ephemeral"}}]}]`, false},
		{"Claude_MCP_assistant_typed", "claude", "tool", `"messages":[{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"SECRET id","content":[{"type":"text","text":%s},{"type":"image","source":{"data":"SECRET image"}},{"type":"unknown","text":"SECRET unknown"}]}]}]`, false},
		{"Claude_system_scalar", "claude", "system", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s}]`, false},
		{"Claude_system_history_scalar", "claude", "system", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s},{"role":"assistant","content":"safe reply"},{"role":"user","content":"continue"}]`, false},
		{"Claude_system_effort_scalar", "claude", "system", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s,"output_config":{"effort":"low"}}]`, false},
		{"Claude_history_scalar", "claude", "assistant", `"messages":[{"role":"assistant","content":%s},{"role":"user","content":"safe"}]`, false},
		{"Claude_document_title", "claude", "user", `"messages":[{"role":"user","content":[{"type":"document","title":%s,"source":{"type":"text","data":"safe"},"citations":{"enabled":true}}]}]`, false},
		{"Claude_nested_document_title", "claude", "tool", `"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"SECRET id","content":[{"type":"document","title":%s,"source":{"type":"text","data":"safe"}}]}]}]`, false},
		{"Chat_tool_carrier", "openai", "tool", `"messages":[{"role":"tool","tool_call_id":"SECRET call","content":%s}]`, true},
		{"Chat_legacy_function_carrier", "openai", "tool", `"messages":[{"role":"function","name":"SECRET name","content":%s}]`, true},
		{"Responses_function_carrier", "openai-response", "tool", `"input":[{"type":"function_call_output","call_id":"SECRET call","output":%s}]`, true},
		{"Responses_custom_carrier", "openai-response", "tool", `"input":[{"type":"custom_tool_call_output","call_id":"SECRET call","output":%s}]`, true},
		{"Responses_function_scalar", "openai-response", "tool", `"input":[{"type":"function_call_output","call_id":"SECRET call","output":%s}]`, false},
		{"Responses_function_typed", "openai-response", "tool", `"input":[{"type":"function_call_output","call_id":"SECRET call","output":[{"type":"input_text","text":%s},{"type":"image_url","image_url":{"url":"SECRET machine"}}]}]`, false},
		{"Responses_shell_stdout", "openai-response", "tool", `"input":[{"type":"shell_call_output","call_id":"SECRET call","output":[{"stdout":%s,"stderr":"safe","exit_code":1}]}]`, false},
		{"Responses_shell_stderr", "openai-response", "tool", `"input":[{"type":"shell_call_output","call_id":"SECRET call","output":[{"stdout":"safe","stderr":%s,"exit_code":1}]}]`, false},
		{"Responses_apply_patch", "openai-response", "tool", `"input":[{"type":"apply_patch_call_output","call_id":"SECRET call","output":%s}]`, false},
		{"Responses_program_result", "openai-response", "tool", `"input":[{"type":"program_output","result":%s,"id":"SECRET id"}]`, false},
		{"decode_Chat", "openai", "user", `"messages":[{"role":"user","content":%s}]`, false},
		{"decode_Responses", "openai-response", "user", `"input":%s`, false},
		{"decode_Claude", "claude", "user", `"messages":[{"role":"user","content":%s}]`, false},
		{"decode_Gemini", "gemini", "user", `"contents":[{"role":"user","parts":[{"text":%s}]}]`, false},
		{"decode_Interactions", "interactions", "user", `"input":%s`, false},
		{"Interactions_caller_annotations", "interactions", "user", `"input":{"type":"text","text":%s,"annotations":[{"type":"url_citation","url":"https://example.test/SECRET"}]}`, false},
	}
	for _, alias := range []string{"response_schema", "_responseJsonSchema", "response_json_schema", "response_json_schema_ordered"} {
		for _, property := range []bool{false, true} {
			schema := `{"description":%s,"enum":["SECRET enum"]}`
			name := "root"
			if property {
				schema = `{"properties":{"SECRET key":{"description":%s,"default":"SECRET default"}}}`
				name = "property"
			}
			fixtures = append(fixtures, functionalFixture{"Gemini_" + alias + "_" + name, "gemini", "developer", `"contents":[{"role":"user","parts":[{"text":"safe"}]}],"generationConfig":{"responseMimeType":"application/json","` + alias + `":` + schema + `}`, false})
		}
	}
	return fixtures
}

func functionalBody(t *testing.T, fixture functionalFixture, text string) []byte {
	t.Helper()
	if fixture.carrier {
		text = `[{"type":"text","text":` + benchmarkQuote(text) + `},{"type":"input_text","text":` + benchmarkQuote(text) + `},{"type":"output_text","text":` + benchmarkQuote(text) + `},{"type":"image_url","image_url":{"url":"https://example.test/SECRET.png"}},{"type":"input_image","file_id":"SECRET file"},{"type":"unknown","text":"SECRET unknown"}]`
	}
	return functionalRawBody(fixture, benchmarkQuote(text))
}

func functionalRawBody(fixture functionalFixture, token string) []byte {
	return []byte(`{"model":"body-decoy","metadata":{"note":"SECRET metadata","n":1.2300e+00},` + fmt.Sprintf(fixture.fragment, token) + `}`)
}

func functionalRequest(format string, body []byte) pluginapi.RequestInterceptRequest {
	metadata := selectedAuthMetadata()
	metadata["source_model"] = "source-decoy"
	metadata["preserved"] = map[string]any{"note": "SECRET metadata", "items": []any{"SECRET", float64(7)}}
	return pluginapi.RequestInterceptRequest{
		RequestID: "functional-after", TraceID: "functional-trace", SourceFormat: format, ToFormat: "upstream-decoy",
		Model: "target-model", RequestedModel: "requested-decoy", Metadata: metadata, Body: body,
		Headers: http.Header{"Content-Encoding": {"identity"}, "Content-Length": {"123"}, "Transfer-Encoding": {"chunked"}, "X-Preserved": {"SECRET header"}},
	}
}

func functionalCall(t *testing.T, method string, request pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse {
	t.Helper()
	before, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := callInterceptRequestAt(method, request)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(request)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("request body/metadata/headers changed, error=%v", err)
	}
	return response
}

func requireFunctionalRewrite(t *testing.T, response pluginapi.RequestInterceptResponse, want []byte) {
	t.Helper()
	expected := pluginapi.RequestInterceptResponse{Body: want, ClearHeaders: []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"}}
	if !reflect.DeepEqual(response, expected) {
		t.Fatalf("rewrite terminate=%t status=%d body bytes=%d want=%d headers=%v clear=%v response headers=%v", response.Terminate, response.StatusCode, len(response.Body), len(want), response.Headers, response.ClearHeaders, response.ResponseHeaders)
	}
}

func requireFunctionalBlock(t *testing.T, response pluginapi.RequestInterceptResponse, term, role string) {
	t.Helper()
	var decoded struct {
		Error struct{ Code, Term, Role string }
	}
	if err := json.Unmarshal(response.ResponseBody, &decoded); err != nil {
		t.Fatal(err)
	}
	if !response.Terminate || response.StatusCode != 400 || response.Body != nil || response.Headers != nil || response.ClearHeaders != nil || response.ResponseHeaders.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != term || decoded.Error.Role != role {
		t.Fatalf("blocked response=%#v", response)
	}
}

func TestFunctionalCorrectionsBothInterceptionPhases(t *testing.T) {
	for _, fixture := range functionalFixtures() {
		for _, phase := range []struct{ name, method, filter string }{
			{"BeforeAuth", pluginabi.MethodRequestInterceptBefore, ""},
			{"AfterAuth", pluginabi.MethodRequestInterceptAfter, "filter_mode: include\nfilter:\n  models: [target-model]\n"},
		} {
			for _, action := range []struct{ name, want string }{{"block", ""}, {"strip", " tail"}, {"obfs", "S​ECRET tail"}} {
				t.Run(fixture.name+"/"+phase.name+"/"+action.name, func(t *testing.T) {
					registerConfig(t, "words:\n  "+action.name+": [SECRET]\nscope:\n  roles: ["+fixture.role+"]\n"+phase.filter)
					request := functionalRequest(fixture.format, functionalBody(t, fixture, "SECRET tail"))
					response := functionalCall(t, phase.method, request)
					if action.name == "block" {
						requireFunctionalBlock(t, response, "SECRET", fixture.role)
					} else {
						requireFunctionalRewrite(t, response, functionalBody(t, fixture, action.want))
					}
				})
			}
			t.Run(fixture.name+"/"+phase.name+"/cancellation", func(t *testing.T) {
				registerConfig(t, "words:\n  strip: ['​']\n  obfs: [ab]\nscope:\n  roles: ["+fixture.role+"]\n"+phase.filter)
				body := functionalBody(t, fixture, "a​b tail")
				requireNoOpResponse(t, functionalCall(t, phase.method, functionalRequest(fixture.format, body)))
			})
		}
		for _, bypass := range []string{"model_BeforeAuth", "model_free_AfterAuth", "unselected_AfterAuth", "filter_miss", "role_disabled", "format_disabled"} {
			t.Run(fixture.name+"/"+bypass, func(t *testing.T) {
				request := functionalRequest(fixture.format, functionalBody(t, fixture, "SECRET tail"))
				method := pluginabi.MethodRequestInterceptAfter
				config := "words:\n  block: [SECRET]\nscope:\n  roles: [" + fixture.role + "]\nfilter_mode: include\nfilter:\n  models: [target-model]\n"
				switch bypass {
				case "model_BeforeAuth":
					method = pluginabi.MethodRequestInterceptBefore
				case "model_free_AfterAuth":
					config = "words:\n  block: [SECRET]\nscope:\n  roles: [" + fixture.role + "]\n"
				case "unselected_AfterAuth":
					request.Metadata = map[string]any{"preserved": "SECRET", "source_model": "target-model"}
				case "filter_miss":
					request.Model = "model-decoy"
					request.RequestedModel = "target-model"
				case "role_disabled":
					disabledRole := "system"
					if fixture.role == "system" {
						disabledRole = "developer"
					}
					config = "words:\n  block: [SECRET]\nscope:\n  roles: [" + disabledRole + "]\nfilter_mode: include\nfilter:\n  models: [target-model]\n"
				case "format_disabled":
					other := "interactions"
					if fixture.format == other {
						other = "openai"
					}
					config = strings.Replace(config, "scope:\n", "scope:\n  formats: ["+other+"]\n", 1)
				}
				registerConfig(t, config)
				requireNoOpResponse(t, functionalCall(t, method, request))
			})
		}
	}
}

func TestFunctionalFlatCustomDescription(t *testing.T) {
	fixture := functionalFixtures()[0]
	if fixture.name != "Chat_flat_custom" {
		t.Fatal("flat custom fixture missing")
	}
	for _, phase := range []struct{ name, method, filter string }{
		{"BeforeAuth", pluginabi.MethodRequestInterceptBefore, ""},
		{"AfterAuth", pluginabi.MethodRequestInterceptAfter, "filter_mode: include\nfilter:\n  models: [target-model]\n"},
	} {
		for _, tc := range []struct{ name, token, words, want string }{
			{"full strip", `"SECRET"`, "  strip: [SECRET]\n", `""`},
			{"no match", `"safe"`, "  block: [SECRET]\n", ""},
			{"cascade once", `"aXb"`, "  strip: [X]\n  obfs: [ab, ab]\n", `"a​b"`},
			{"escaped cancellation", `"\u0061\u200bb"`, "  strip: ['​']\n  obfs: [ab]\n", ""},
			{"folded", `"secret tail"`, "  obfs: [SECRET]\n", `"s​ecret tail"`},
			{"Unicode", `"界SECRET😀"`, "  strip: [SECRET]\n", `"界😀"`},
			{"escaped Unicode", `"\u0053ECRET\u754c"`, "  obfs: [SECRET]\n", `"S​ECRET界"`},
			{"surrogate subsequent escape", `"\ud800\u0053ECRET tail"`, "  strip: [SECRET]\n", `"� tail"`},
		} {
			t.Run(phase.name+"/"+tc.name, func(t *testing.T) {
				config := "words:\n" + tc.words + "scope:\n  roles: [developer]\n" + phase.filter
				if tc.name == "folded" {
					config += "ignore_case: true\n"
				}
				registerConfig(t, config)
				body := functionalRawBody(fixture, tc.token)
				response := functionalCall(t, phase.method, functionalRequest("openai", body))
				if tc.want == "" {
					requireNoOpResponse(t, response)
				} else {
					requireFunctionalRewrite(t, response, replaceRawTokens(t, body, rawReplacement{Before: tc.token, After: tc.want}))
				}
			})
		}
		for _, action := range []struct{ name, want string }{{"block", ""}, {"strip", " tail"}, {"obfs", "s​ecret tail"}} {
			t.Run(phase.name+"/folded_"+action.name, func(t *testing.T) {
				registerConfig(t, "words:\n  "+action.name+": [SECRET]\nignore_case: true\n"+phase.filter)
				response := functionalCall(t, phase.method, functionalRequest("openai", functionalBody(t, fixture, "secret tail")))
				if action.name == "block" {
					requireFunctionalBlock(t, response, "SECRET", "developer")
				} else {
					requireFunctionalRewrite(t, response, functionalBody(t, fixture, action.want))
				}
			})
		}
		for _, tc := range []struct{ name, body, words, term, role string }{
			{"flat earliest", `{"tools":[{"type":"custom","name":"safe","description":"SECRET"}],"messages":[{"role":"user","content":"SECRET"}]}`, "  block: [SECRET]\n", "SECRET", "developer"},
			{"user earliest", `{"messages":[{"role":"user","content":"SECRET"}],"tools":[{"type":"custom","name":"safe","description":"SECRET"}]}`, "  block: [SECRET]\n", "SECRET", "user"},
			{"rule major", `{"tools":[{"type":"custom","name":"safe","description":"EARLY"}],"messages":[{"role":"user","content":"LATE"}]}`, "  block: [LATE, EARLY]\n", "LATE", "user"},
			{"no cross node", `{"tools":[{"type":"custom","name":"safe","description":"SEC","custom":{"description":"RET"}}]}`, "  block: [SECRET]\n", "", ""},
			{"block before strip and obfs", `{"tools":[{"type":"custom","name":"safe","description":"SECRET"}]}`, "  obfs: [SECRET]\n  strip: [SECRET]\n  block: [SECRET]\n", "SECRET", "developer"},
		} {
			t.Run(phase.name+"/"+tc.name, func(t *testing.T) {
				registerConfig(t, "words:\n"+tc.words+phase.filter)
				response := functionalCall(t, phase.method, functionalRequest("openai", []byte(tc.body)))
				if tc.term == "" {
					requireNoOpResponse(t, response)
				} else {
					requireFunctionalBlock(t, response, tc.term, tc.role)
				}
			})
		}
	}
	t.Run("empty request Model ignores all decoys", func(t *testing.T) {
		registerConfig(t, "words:\n  block: [SECRET]\nfilter_mode: include\nfilter:\n  models: [target-model]\n")
		request := functionalRequest("openai", functionalBody(t, fixture, "SECRET tail"))
		request.Model, request.RequestedModel = "", "target-model"
		request.Metadata["source_model"] = "target-model"
		request.Body = bytes.Replace(request.Body, []byte("body-decoy"), []byte("target-model"), 1)
		request.Headers.Set("X-Model", "target-model")
		requireNoOpResponse(t, functionalCall(t, pluginabi.MethodRequestInterceptAfter, request))
	})
	t.Run("model BeforeAuth bypasses malformed envelope", func(t *testing.T) {
		registerConfig(t, "words:\n  block: [SECRET]\nfilter:\n  models: [target-model]\n")
		raw, err := handleMethod(pluginabi.MethodRequestInterceptBefore, []byte(`{"invalid envelope`))
		var envelope pluginabi.Envelope
		if err != nil || json.Unmarshal(raw, &envelope) != nil || !envelope.OK {
			t.Fatalf("raw=%s error=%v; want successful response", raw, err)
		}
		var response pluginapi.RequestInterceptResponse
		if err := json.Unmarshal(envelope.Result, &response); err != nil {
			t.Fatal(err)
		}
		requireNoOpResponse(t, response)
	})
}

func TestFunctionalCorrectionsRawSurrogateBothInterceptionPhases(t *testing.T) {
	for _, fixture := range functionalFixtures() {
		if !strings.HasPrefix(fixture.name, "decode_") {
			continue
		}
		const escape = `\u`
		for _, token := range []string{`"` + escape + `d800` + escape + `0065xample note"`, `"` + escape + `dc00` + escape + `0065xample note"`} {
			var original string
			if err := json.Unmarshal([]byte(token), &original); err != nil {
				t.Fatal(err)
			}
			for _, phase := range []struct{ name, method, filter string }{
				{"BeforeAuth", pluginabi.MethodRequestInterceptBefore, ""},
				{"AfterAuth", pluginabi.MethodRequestInterceptAfter, "filter_mode: include\nfilter:\n  models: [target-model]\n"},
			} {
				for _, action := range []struct{ name, mode, term, replacement string }{
					{"block", "block", "example", ""},
					{"strip", "strip", "example", ""},
					{"obfs", "obfs", "example", "e​xample"},
					{"strip_other", "strip", "note", ""},
					{"obfs_other", "obfs", "note", "n​ote"},
					{"no_match", "strip", "MISS", ""},
				} {
					t.Run(fixture.name+"/"+token+"/"+phase.name+"/"+action.name, func(t *testing.T) {
						registerConfig(t, "words:\n  "+action.mode+": ["+action.term+"]\nscope:\n  roles: [user]\n"+phase.filter)
						// 原始 escape 直接进入 RPC，只有期望输出由 encoding/json 解码。
						body := functionalRawBody(fixture, token)
						request := functionalRequest(fixture.format, body)
						if phase.name == "AfterAuth" {
							requireNoOpResponse(t, functionalCall(t, pluginabi.MethodRequestInterceptBefore, request))
							unselected := request
							unselected.Metadata = map[string]any{"source_model": "target-model"}
							requireNoOpResponse(t, functionalCall(t, phase.method, unselected))
							miss := request
							miss.Model, miss.RequestedModel = "model-decoy", "target-model"
							requireNoOpResponse(t, functionalCall(t, phase.method, miss))
						}
						response := functionalCall(t, phase.method, request)
						if action.name == "no_match" {
							requireNoOpResponse(t, response)
						} else if action.mode == "block" {
							requireFunctionalBlock(t, response, action.term, "user")
						} else {
							want := strings.ReplaceAll(original, action.term, action.replacement)
							requireFunctionalRewrite(t, response, functionalBody(t, fixture, want))
						}
					})
				}
			}
		}
	}
}

func TestFunctionalCorrectionsNonEmptyAndLimitsBothPhases(t *testing.T) {
	for _, method := range []string{pluginabi.MethodRequestInterceptBefore, pluginabi.MethodRequestInterceptAfter} {
		filter := ""
		if method == pluginabi.MethodRequestInterceptAfter {
			filter = "filter_mode: include\nfilter:\n  models: [target-model]\n"
		}
		for _, tc := range []struct {
			name, fragment, role string
			invalid              bool
		}{
			{"system_scalar", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s}]`, "system", true},
			{"system_history_scalar", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s},{"role":"assistant","content":"safe reply"},{"role":"user","content":"continue"}]`, "system", true},
			{"system_typed", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":[{"type":"text","text":%s}]}]`, "system", true},
			{"system_effort_typed", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":[{"type":"text","text":%s}],"output_config":{"effort":"low"}}]`, "system", true},
			{"system_effort_null", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s,"output_config":{"effort":null}}]`, "system", false},
			{"system_effort_empty", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s,"output_config":{"effort":""}}]`, "system", false},
			{"system_effort_nonstring", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s,"output_config":{"effort":false}}]`, "system", false},
			{"system_output_config_empty", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s,"output_config":{}}]`, "system", true},
			{"system_effort_scalar", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":%s,"output_config":{"effort":"low"}}]`, "system", false},
			{"top_level_system", `"system":%s,"messages":[{"role":"user","content":"safe"}]`, "system", false},
			{"tool_scalar", `"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":%s}]}]`, "tool", false},
			{"history_scalar", `"messages":[{"role":"assistant","content":%s},{"role":"user","content":"safe"}]`, "assistant", true},
			{"last_scalar", `"messages":[{"role":"user","content":"safe"},{"role":"assistant","content":%s}]`, "assistant", false},
			{"history_before_null", `"messages":[{"role":"assistant","content":%s},null]`, "assistant", true},
			{"MCP_scalar", `"messages":[{"role":"assistant","content":[{"type":"mcp_tool_result","content":%s}]}]`, "tool", false},
			{"MCP_typed", `"messages":[{"role":"assistant","content":[{"type":"mcp_tool_result","content":[{"type":"text","text":%s}]}]}]`, "tool", true},
			{"document_title", `"messages":[{"role":"user","content":[{"type":"document","title":%s}]}]`, "user", true},
			{"nested_document_title", `"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"document","title":%s}]}]}]`, "tool", true},
		} {
			t.Run(method+"/"+tc.name+"/empty", func(t *testing.T) {
				fixture := functionalFixture{format: "claude", role: tc.role, fragment: tc.fragment}
				registerConfig(t, "words:\n  strip: [SECRET]\nscope:\n  roles: ["+tc.role+"]\n"+filter)
				response := functionalCall(t, method, functionalRequest("claude", functionalBody(t, fixture, "SECRET")))
				if tc.invalid {
					requireOpenAIInvalidRewrite(t, response)
				} else {
					requireFunctionalRewrite(t, response, functionalBody(t, fixture, ""))
				}
			})
		}
		for _, fixture := range functionalFixtures() {
			limit := 0
			switch fixture.name {
			case "Claude_document_title", "Claude_nested_document_title":
				limit = 500
			case "Responses_function_scalar", "Responses_function_typed", "Responses_shell_stdout", "Responses_shell_stderr", "Responses_apply_patch", "Responses_program_result":
				limit = 10485760
			}
			if limit == 0 {
				continue
			}
			for _, length := range []int{limit - 1, limit} {
				t.Run(fmt.Sprintf("%s/%s/final_%d", method, fixture.name, length+1), func(t *testing.T) {
					registerConfig(t, "words:\n  obfs: [ab]\nscope:\n  roles: ["+fixture.role+"]\n"+filter)
					text := strings.Repeat("x", length-2) + "ab"
					response := functionalCall(t, method, functionalRequest(fixture.format, functionalBody(t, fixture, text)))
					if length == limit {
						requireOpenAIInvalidRewrite(t, response)
					} else {
						requireFunctionalRewrite(t, response, functionalBody(t, fixture, strings.Repeat("x", length-2)+"a​b"))
					}
				})
			}
		}
	}
}

func TestClaudeSystemScalarFinalTextBothPhases(t *testing.T) {
	for _, fixture := range functionalFixtures() {
		if !strings.HasPrefix(fixture.name, "Claude_system_") {
			continue
		}
		for _, method := range []string{pluginabi.MethodRequestInterceptBefore, pluginabi.MethodRequestInterceptAfter} {
			for _, tc := range []struct{ name, config, input, outcome string }{
				{"fold_empty", "words:\n  strip: [SECRET]\nignore_case: true\n", "secret", "invalid"},
				{"cascade_empty", "words:\n  strip: [X, SECRET]\n", "SEXCXRET", "invalid"},
				{"block_precedes_empty", "words:\n  block: [SECRET]\n  strip: [SECRET]\n", "SECRET", "block"},
				{"no_match", "words:\n  strip: [SECRET]\n", "safe", "no_op"},
			} {
				t.Run(fixture.name+"/"+method+"/"+tc.name, func(t *testing.T) {
					config := tc.config
					if method == pluginabi.MethodRequestInterceptAfter {
						config += "filter_mode: include\nfilter:\n  models: [target-model]\n"
					}
					registerConfig(t, config)
					response := functionalCall(t, method, functionalRequest("claude", functionalBody(t, fixture, tc.input)))
					switch tc.outcome {
					case "invalid":
						if fixture.name == "Claude_system_effort_scalar" {
							requireFunctionalRewrite(t, response, functionalBody(t, fixture, ""))
							return
						}
						requireOpenAIInvalidRewrite(t, response)
						if string(response.ResponseBody) != `{"error":{"type":"invalid_request_error","code":"censorship_invalid_request","message":"censorship rewrite would make a text field invalid"}}` {
							t.Fatalf("invalid-text response=%s", response.ResponseBody)
						}
					case "block":
						requireFunctionalBlock(t, response, "SECRET", "system")
					case "no_op":
						requireNoOpResponse(t, response)
					}
				})
			}
		}
	}
}

func TestClaudeSystemScalarUnchangedBoundariesBothPhases(t *testing.T) {
	for _, method := range []string{pluginabi.MethodRequestInterceptBefore, pluginabi.MethodRequestInterceptAfter} {
		for _, tc := range []struct{ name, config, fragment, want string }{
			{"roles_empty", "scope:\n  roles: []\n", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":"SECRET"}]`, ""},
			{"formats_empty", "scope:\n  formats: []\n", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":"SECRET"}]`, ""},
			{"rules_empty", "words: {}\n", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":"SECRET"}]`, ""},
			{"effort_only", "", `"messages":[{"role":"system","content":[],"output_config":{"effort":"low"}},{"role":"user","content":"safe"}]`, ""},
			{"existing_empty", "", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":""}]`, ""},
			{"empty_beside_rewrite", "", `"system":"SECRET","messages":[{"role":"user","content":"safe"},{"role":"system","content":""}]`, `"system":"","messages":[{"role":"user","content":"safe"},{"role":"system","content":""}]`},
			{"unknown_role", "", `"messages":[{"role":"unknown","content":"SECRET"}]`, ""},
			{"unknown_block", "", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":[{"type":"unknown","text":"SECRET"}]}]`, ""},
			{"unknown_shape", "", `"messages":[{"role":"user","content":"safe"},{"role":"system","content":{"text":"SECRET"}}]`, ""},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				config := "words:\n  strip: [SECRET]\n" + tc.config
				if tc.name == "rules_empty" {
					config = tc.config
				}
				if method == pluginabi.MethodRequestInterceptAfter {
					config += "filter_mode: include\nfilter:\n  models: [target-model]\n"
				}
				registerConfig(t, config)
				body := []byte(`{"model":"claude-opus-5-5",` + tc.fragment + `}`)
				response := functionalCall(t, method, functionalRequest("claude", body))
				if tc.want == "" {
					requireNoOpResponse(t, response)
				} else {
					requireFunctionalRewrite(t, response, []byte(`{"model":"claude-opus-5-5",`+tc.want+`}`))
				}
			})
		}
	}
}

func TestFunctionalCorrectionsProtectedTextAndOrderingBothPhases(t *testing.T) {
	for _, method := range []string{pluginabi.MethodRequestInterceptBefore, pluginabi.MethodRequestInterceptAfter} {
		filter := ""
		if method == pluginabi.MethodRequestInterceptAfter {
			filter = "filter_mode: include\nfilter:\n  models: [target-model]\n"
		}
		for _, tc := range []struct{ name, format, role, body string }{
			{"signature", "gemini", "user", `{"contents":[{"role":"user","parts":[{"text":"SECRET tail","thoughtSignature":"SECRET signature"}]}]}`},
			{"annotations", "openai-response", "assistant", `{"input":[{"role":"assistant","content":[{"type":"output_text","text":"SECRET tail","annotations":[{"type":"url_citation","start_index":0,"end_index":6,"url":"https://example.test/SECRET"}]}]}]}`},
			{"interactions_annotations", "interactions", "assistant", `{"input":[{"type":"model_output","content":[{"type":"text","text":"SECRET tail","annotations":[{"type":"url_citation","url":"https://example.test/SECRET"}]}]}]}`},
		} {
			for _, action := range []string{"block", "strip", "obfs"} {
				t.Run(method+"/"+tc.name+"/"+action, func(t *testing.T) {
					registerConfig(t, "words:\n  "+action+": [SECRET]\nscope:\n  roles: ["+tc.role+"]\n"+filter)
					response := functionalCall(t, method, functionalRequest(tc.format, []byte(tc.body)))
					if action == "block" {
						requireFunctionalBlock(t, response, "SECRET", tc.role)
					} else {
						requireOpenAIInvalidRewrite(t, response)
					}
				})
			}
		}
		t.Run(method+"/rule_major_then_outer_token", func(t *testing.T) {
			inner := `[{"type":"text","text":"EARLY"},{"type":"input_image","file_id":"SECRET file"}]`
			body := []byte(`{"messages":[{"role":"tool","content":` + benchmarkQuote(inner) + `},{"role":"user","content":"LATE"},{"role":"tool","content":` + benchmarkQuote(strings.Replace(inner, "EARLY", "LATE", 1)) + `}]}`)
			registerConfig(t, "words:\n  block: [LATE, EARLY]\nscope:\n  roles: [user, tool]\n"+filter)
			requireFunctionalBlock(t, functionalCall(t, method, functionalRequest("openai", body)), "LATE", "user")
		})
		t.Run(method+"/dormant_description", func(t *testing.T) {
			body := []byte(`{"tools":[{"type":"function","description":"SECRET","function":{"description":"BLOCK nested","name":"SECRET machine"}}]}`)
			registerConfig(t, "words:\n  block: [BLOCK]\n  strip: [SECRET]\nscope:\n  roles: [developer]\n"+filter)
			requireFunctionalBlock(t, functionalCall(t, method, functionalRequest("openai-response", body)), "BLOCK", "developer")
		})
	}
}
