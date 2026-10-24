//go:build integration

package censorshipintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/tidwall/gjson"
)

func geminiTypedSchemaCases() []geminiGenerationConfigCase {
	return append(geminiGenerationConfigCases(), geminiGenerationConfigCase{
		name: "json_schema_any_of", config: "words:\n  block: [SECRET]\n", input: "SECRET answer", want: "SECRET answer", noOp: true,
	})
}

func geminiTypedSchemaFixtures(tc geminiGenerationConfigCase) []functionalHTTPFixture {
	var roots []struct{ name, prefix, fragment string }
	for _, declarations := range []string{"functionDeclarations", "function_declarations"} {
		for _, key := range []string{"parameters", "response"} {
			roots = append(roots, struct{ name, prefix, fragment string }{declarations + "/" + key, "tools.0." + declarations + ".0." + key, `"tools":[{"` + declarations + `":[{"name":"SECRET_lookup","` + key + `":%s}]}]`})
		}
	}
	for _, root := range []string{"generationConfig", "generation_config"} {
		for _, key := range []string{"responseSchema", "response_schema"} {
			roots = append(roots, struct{ name, prefix, fragment string }{root + "/" + key, root + "." + key, `"` + root + `":{"responseMimeType":"application/json","` + key + `":%s,"temperature":1.2300e+00,"stopSequences":["SECRET stop"]}`})
		}
	}
	if tc.name == "json_schema_any_of" {
		roots = []struct{ name, prefix, fragment string }{{"generation_config/response_json_schema", "generation_config.response_json_schema", `"generation_config":{"responseMimeType":"application/json","response_json_schema":%s,"temperature":1.2300e+00,"stopSequences":["SECRET stop"]}`}}
	} else if tc.noOp {
		roots = roots[:1]
	}
	var fixtures []functionalHTTPFixture
	for _, root := range roots {
		keywords := []string{"anyOf", "any_of"}
		if tc.name == "json_schema_any_of" {
			keywords = []string{"any_of"}
		}
		for _, keyword := range keywords {
			branch := root.prefix + ".properties.any_of.items." + keyword + ".0"
			schema := `{"type":"OBJECT","properties":{"any_of":{"type":"ARRAY","items":{"` + keyword + `":[{"type":"STRING","description":%[1]s,"enum":["SECRET enum"],"default":"SECRET default","example":{"description":"SECRET example"},"metadata":{"any_of":[{"description":"SECRET hidden"}]}}]}}},"required":["any_of"],"propertyOrdering":["any_of"]}`
			machine := map[string]string{
				root.prefix + ".type": `"OBJECT"`, root.prefix + ".properties.any_of.type": `"ARRAY"`,
				branch + ".type": `"STRING"`, branch + ".enum": `["SECRET enum"]`, branch + ".default": `"SECRET default"`,
				branch + ".example": `{"description":"SECRET example"}`, branch + ".metadata": `{"any_of":[{"description":"SECRET hidden"}]}`,
				root.prefix + ".required": `["any_of"]`, root.prefix + ".propertyOrdering": `["any_of"]`,
				"metadata": `{"description":"SECRET metadata","n":1.2300e+00}`,
			}
			if strings.HasPrefix(root.prefix, "tools.") {
				machine[root.prefix[:strings.LastIndex(root.prefix, ".")]+".name"] = `"SECRET_lookup"`
			} else {
				config := strings.Split(root.prefix, ".")[0]
				machine[config+".responseMimeType"] = `"application/json"`
				machine[config+".temperature"] = `1.2300e+00`
				machine[config+".stopSequences"] = `["SECRET stop"]`
			}
			fixtures = append(fixtures, functionalHTTPFixture{
				name: root.name + "/" + keyword, format: "gemini", provider: "gemini", role: "developer", endpoint: "/v1beta/models/censorship-integration-model:generateContent",
				fragment: fmt.Sprintf(root.fragment, schema) + `,"contents":[{"role":"user","parts":[{"text":"safe canary"}]}],"metadata":{"description":"SECRET metadata","n":1.2300e+00}`,
				textPath: branch + ".description", machine: machine,
			})
		}
	}
	return fixtures
}

func requireGeminiTypedSchemaWire(t *testing.T, fixture functionalHTTPFixture, body []byte, want string, converted bool) {
	t.Helper()
	path := func(source string) string {
		if !converted {
			return source
		}
		// 固定 converter 搬移原 schema 对象，内部 any_of 不归一化。
		source = strings.Replace(source, ".functionDeclarations.", ".function_declarations.", 1)
		source = strings.Replace(source, ".parameters.", ".parametersJsonSchema.", 1)
		return strings.Replace(source, "generationConfig.responseSchema.", "generationConfig.responseJsonSchema.", 1)
	}
	if got := gjson.GetBytes(body, path(fixture.textPath)); got.Type != gjson.String || got.Str != want {
		t.Errorf("description %s=%s, want %q; body=%s", path(fixture.textPath), got.Raw, want, body)
	}
	for field, wantRaw := range fixture.machine {
		if got := gjson.GetBytes(body, path(field)).Raw; got != wantRaw {
			t.Errorf("machine %s=%s, want raw %s", path(field), got, wantRaw)
		}
	}
	if converted {
		for _, old := range []string{"tools.0.functionDeclarations", "tools.0.function_declarations.0.parameters", "generationConfig.responseSchema"} {
			if gjson.GetBytes(body, old).Exists() {
				t.Errorf("fixed converter left source field %s", old)
			}
		}
	}
}

func TestGeminiTypedSchemaNative(t *testing.T) {
	const childKey = "CENSORSHIP_GEMINI_TYPED_CASE"
	child := os.Getenv(childKey)
	for _, phase := range []string{"BeforeAuth", "AfterAuth"} {
		for _, tc := range geminiTypedSchemaCases() {
			name := phase + "/" + tc.name
			if child != "" && child != name {
				continue
			}
			t.Run(name, func(t *testing.T) {
				if child == "" {
					binary, err := os.Executable()
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
					defer cancel()
					cmd := exec.CommandContext(ctx, binary, "-test.run=^TestGeminiTypedSchemaNative$", "-test.v")
					cmd.Env = append(os.Environ(), childKey+"="+name)
					output, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("fresh native child: %v\n%s", err, output)
					}
					t.Logf("fresh native child %s\n%s", name, output)
					return
				}
				config := geminiGenerationConfigYAML(tc, phase, "target-model")
				indented := "      " + strings.ReplaceAll(strings.TrimSuffix(config, "\n"), "\n", "\n      ") + "\n"
				t.Logf("native plugin directory=%s", os.Getenv("CENSORSHIP_PLUGIN_DIR"))
				withDynamicABIHost(t, indented, func(before, after func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse) {
					for _, fixture := range geminiTypedSchemaFixtures(tc) {
						t.Run(fixture.name, func(t *testing.T) {
							body := functionalHTTPBody(t, fixture, tc.input)
							headers := http.Header{"Content-Encoding": {"identity"}, "Content-Length": {"123"}, "Transfer-Encoding": {"chunked"}, "X-Preserved": {"SECRET header"}}
							request := pluginapi.RequestInterceptRequest{SourceFormat: "gemini", Model: "target-model", RequestedModel: "requested-decoy", Body: bytes.Clone(body), Headers: headers.Clone(), Metadata: map[string]any{executor.SelectedAuthMetadataKey: "auth-1", "preserved": "SECRET metadata"}}
							original, err := json.Marshal(request)
							if err != nil {
								t.Fatal(err)
							}
							unchanged := pluginapi.RequestInterceptResponse{Body: body, Headers: headers}
							call, inactive := before, after
							if phase == "AfterAuth" {
								call, inactive = after, before
								unselected := request
								unselected.Metadata = map[string]any{"source_model": "target-model"}
								if got := after(unselected); !reflect.DeepEqual(got, unchanged) {
									t.Fatalf("missing selected-auth marker=%#v", got)
								}
								miss := request
								miss.Model, miss.RequestedModel = "model-decoy", "target-model"
								miss.Body = bytes.Replace(body, []byte(modelName), []byte("target-model"), 1)
								if got := after(miss); !reflect.DeepEqual(got, pluginapi.RequestInterceptResponse{Body: miss.Body, Headers: headers}) {
									t.Fatalf("filter miss with opposite decoys=%#v", got)
								}
							}
							if got := inactive(request); !reflect.DeepEqual(got, unchanged) {
								t.Fatalf("inactive phase=%#v", got)
							}
							response := call(request)
							final, err := json.Marshal(request)
							if err != nil || !bytes.Equal(original, final) {
								t.Fatalf("native input/metadata/headers changed: %v", err)
							}
							if tc.block {
								decoded, err := decodeCensorshipError(response.ResponseBody)
								if err != nil || !response.Terminate || response.StatusCode != 400 || !bytes.Equal(response.Body, body) || !reflect.DeepEqual(response.Headers, headers) || response.ClearHeaders != nil || response.ResponseHeaders.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != "SECRET" || decoded.Error.Role != "developer" {
									t.Fatalf("native block=%#v error=%v", response, err)
								}
							} else if tc.noOp {
								if !reflect.DeepEqual(response, unchanged) {
									t.Fatalf("native no-op=%#v", response)
								}
							} else {
								want := pluginapi.RequestInterceptResponse{Body: functionalHTTPBody(t, fixture, tc.want), Headers: http.Header{"X-Preserved": {"SECRET header"}}}
								if !reflect.DeepEqual(response, want) {
									t.Fatalf("native rewrite=%s, want=%s", response.Body, want.Body)
								}
								requireGeminiTypedSchemaWire(t, fixture, response.Body, tc.want, false)
							}
						})
					}
				})
			})
		}
	}
}

func TestGeminiTypedSchemaHTTP(t *testing.T) {
	for _, phase := range []string{"BeforeAuth", "AfterAuth"} {
		for _, tc := range geminiTypedSchemaCases() {
			t.Run(phase+"/"+tc.name, func(t *testing.T) {
				upstream := newFunctionalDirectUpstream(t, "gemini")
				cpa := startFunctionalDirectCPA(t, upstream.URL, "gemini", geminiGenerationConfigYAML(tc, phase, modelName))
				defer func() { t.Logf("Gemini typed %s/%s CPA log\n%s", phase, tc.name, readCPALog(cpa.logPath)) }()
				fixtures := geminiTypedSchemaFixtures(tc)
				status, _, response := postJSON(t, cpa.baseURL+fixtures[0].endpoint, functionalHTTPBody(t, fixtures[0], "clean canary"))
				if status != 200 || upstream.arrivalCount() != 1 || upstream.requestCount() != 1 {
					t.Fatalf("clean canary status=%d body=%s arrivals=%d requests=%d", status, response, upstream.arrivalCount(), upstream.requestCount())
				}
				for _, fixture := range fixtures {
					t.Run(fixture.name, func(t *testing.T) {
						body := functionalHTTPBody(t, fixture, tc.input)
						original := bytes.Clone(body)
						arrivals, requests := upstream.arrivalCount(), upstream.requestCount()
						status, header, response := postJSON(t, cpa.baseURL+fixture.endpoint, body)
						if !bytes.Equal(body, original) {
							t.Fatal("HTTP input changed")
						}
						if tc.block {
							decoded, err := decodeCensorshipError(response)
							if err != nil || status != 400 || header.Get("Content-Type") != "application/json" || decoded.Error.Code != "censorship_blocked" || decoded.Error.Term != "SECRET" || decoded.Error.Role != "developer" || upstream.arrivalCount() != arrivals || upstream.requestCount() != requests {
								t.Fatalf("block status=%d arrivals delta=%d requests delta=%d body=%s error=%v", status, upstream.arrivalCount()-arrivals, upstream.requestCount()-requests, response, err)
							}
						} else {
							if status != 200 || upstream.arrivalCount() != arrivals+1 || upstream.requestCount() != requests+1 {
								t.Fatalf("forward status=%d arrivals delta=%d requests delta=%d body=%s", status, upstream.arrivalCount()-arrivals, upstream.requestCount()-requests, response)
							}
							requireGeminiTypedSchemaWire(t, fixture, upstream.lastRequest(), tc.want, true)
						}
					})
				}
			})
		}
	}
}
