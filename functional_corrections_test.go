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

type functionalFixture struct {
	name, format, role, fragment string
	carrier                      bool
}

func functionalFixtures() []functionalFixture {
	fixtures := []functionalFixture{
		{"Chat_schema_root", "openai", "developer", `"messages":[{"role":"user","content":"safe"}],"tools":[{"type":"function","function":{"name":"SECRET name","parametersJsonSchema":{"description":%s,"properties":{"SECRET key":{"enum":["SECRET enum"],"default":"SECRET default"}}}}}]`, false},
		{"Chat_schema_property", "openai", "developer", `"messages":[],"tools":[{"type":"function","function":{"name":"SECRET name","parametersJsonSchema":{"properties":{"SECRET key":{"description":%s,"const":"SECRET const"}}}}}]`, false},
		{"Responses_format_root", "openai-response", "developer", `"input":"safe","text":{"format":{"type":"json_schema","name":"SECRET name","json_schema":{"schema":{"description":%s,"enum":["SECRET enum"]}}}}`, false},
		{"Responses_format_property", "openai-response", "developer", `"input":"safe","text":{"format":{"type":"json_schema","json_schema":{"schema":{"properties":{"SECRET key":{"description":%s,"default":"SECRET default"}}}}}}`, false},
		{"Claude_MCP_assistant_scalar", "claude", "tool", `"messages":[{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"SECRET id","is_error":false,"content":%s,"cache_control":{"type":"ephemeral"}}]}]`, false},
		{"Claude_MCP_assistant_typed", "claude", "tool", `"messages":[{"role":"assistant","content":[{"type":"mcp_tool_result","tool_use_id":"SECRET id","content":[{"type":"text","text":%s},{"type":"image","source":{"data":"SECRET image"}},{"type":"unknown","text":"SECRET unknown"}]}]}]`, false},
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
		text = `[{"type":"text","text":` + openAIStringToken(t, text) + `},{"type":"input_text","text":` + openAIStringToken(t, text) + `},{"type":"output_text","text":` + openAIStringToken(t, text) + `},{"type":"image_url","image_url":{"url":"https://example.test/SECRET.png"}},{"type":"input_image","file_id":"SECRET file"},{"type":"unknown","text":"SECRET unknown"}]`
	}
	return []byte(`{"model":"body-decoy","metadata":{"note":"SECRET metadata","n":1.2300e+00},` + fmt.Sprintf(fixture.fragment, openAIStringToken(t, text)) + `}`)
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
					config = "words:\n  block: [SECRET]\nscope:\n  roles: [system]\nfilter_mode: include\nfilter:\n  models: [target-model]\n"
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
			body := []byte(`{"messages":[{"role":"tool","content":` + openAIStringToken(t, inner) + `},{"role":"user","content":"LATE"},{"role":"tool","content":` + openAIStringToken(t, strings.Replace(inner, "EARLY", "LATE", 1)) + `}]}`)
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
