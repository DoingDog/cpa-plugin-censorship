//go:build integration

package censorshipintegration

import (
	"bytes"
	"context"
	"encoding/json"
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

func geminiGenerationConfigFixtures() []functionalHTTPFixture {
	var fixtures []functionalHTTPFixture
	for _, root := range []string{"generationConfig", "generation_config"} {
		for _, alias := range []string{"responseSchema", "response_schema", "_responseJsonSchema", "response_json_schema", "responseJsonSchema", "response_json_schema_ordered", "responseFormat.text.schema"} {
			objectType, stringType := `"object"`, `"string"`
			if alias == "responseSchema" || alias == "response_schema" {
				objectType, stringType = `"OBJECT"`, `"STRING"`
			}
			schema := `{"type":` + objectType + `,"description":%[1]s,"properties":{"SECRET_key":{"type":` + stringType + `,"description":%[1]s,"enum":["SECRET enum"],"const":"SECRET const","default":"SECRET default"}},"required":["SECRET_key"],"metadata":{"description":"SECRET schema metadata"}}`
			field := `"` + alias + `":` + schema
			if alias == "responseFormat.text.schema" {
				field = `"responseFormat":{"text":{"mimeType":"application/json","schema":` + schema + `}}`
			}
			prefix := root + "." + alias
			fixtures = append(fixtures, functionalHTTPFixture{
				name: root + "/" + alias, format: "gemini", provider: "gemini", endpoint: "/v1beta/models/censorship-integration-model:generateContent", role: "developer",
				fragment: `"contents":[{"role":"user","parts":[{"text":"safe canary"}]}],"` + root + `":{"responseMimeType":"application/json",` + field + `,"stopSequences":["SECRET stop"],"temperature":1.2300e+00,"metadata":{"description":"SECRET config metadata"}},"metadata":{"description":"SECRET request metadata","n":1.2300e+00}`,
				textPath: prefix + ".description",
				machine: map[string]string{
					prefix + ".type": objectType, prefix + ".properties.SECRET_key.type": stringType,
					prefix + ".properties.SECRET_key.enum": `["SECRET enum"]`, prefix + ".properties.SECRET_key.const": `"SECRET const"`, prefix + ".properties.SECRET_key.default": `"SECRET default"`,
					prefix + ".required": `["SECRET_key"]`, prefix + ".metadata": `{"description":"SECRET schema metadata"}`,
					root + ".responseMimeType": `"application/json"`, root + ".stopSequences": `["SECRET stop"]`, root + ".temperature": `1.2300e+00`,
					root + ".metadata": `{"description":"SECRET config metadata"}`, "metadata": `{"description":"SECRET request metadata","n":1.2300e+00}`,
				},
			})
		}
	}
	return fixtures
}

type geminiGenerationConfigCase struct {
	name, config, input, want string
	block, noOp               bool
}

func geminiGenerationConfigCases() []geminiGenerationConfigCase {
	return []geminiGenerationConfigCase{
		{"block", "words:\n  block: [SECRET]\n", "SECRET answer", "", true, false},
		{"strip", "words:\n  strip: [SECRET]\n", "SECRET answer", " answer", false, false},
		{"obfs", "words:\n  obfs: [SECRET]\n", "SECRET answer", "S​ECRET answer", false, false},
		{"developer_disabled", "words:\n  block: [SECRET]\nscope:\n  roles: [user]\n", "SECRET answer", "SECRET answer", false, true},
		{"format_disabled", "words:\n  block: [SECRET]\nscope:\n  formats: [openai]\n", "SECRET answer", "SECRET answer", false, true},
		{"filter_bypass", "words:\n  block: [SECRET]\nfilter_mode: include\nfilter:\n  models: [never-match]\n", "SECRET answer", "SECRET answer", false, true},
		{"machine_only", "words:\n  block: [SECRET]\n", "safe answer", "safe answer", false, true},
		{"no_match", "words:\n  block: [ABSENT]\n", "SECRET answer", "SECRET answer", false, true},
		{"cancellation", "words:\n  strip: ['​']\n  obfs: [ab]\n", "a​b answer", "a​b answer", false, true},
	}
}

func geminiGenerationConfigYAML(tc geminiGenerationConfigCase, phase, model string) string {
	config := tc.config
	if phase == "AfterAuth" && tc.name != "filter_bypass" {
		config += "filter_mode: include\nfilter:\n  models: [" + model + "]\n"
	}
	return config
}

func requireGeminiGenerationConfigWire(t *testing.T, fixture functionalHTTPFixture, captured []byte, want string, converted bool) {
	t.Helper()
	path := func(source string) string {
		// 固定 converter 仅改名 camel responseSchema，snake root 保留源路径。
		if converted && strings.HasPrefix(source, "generationConfig.responseSchema.") {
			return strings.Replace(source, "generationConfig.responseSchema.", "generationConfig.responseJsonSchema.", 1)
		}
		return source
	}
	prefix := strings.TrimSuffix(fixture.textPath, ".description")
	for _, description := range []string{fixture.textPath, prefix + ".properties.SECRET_key.description"} {
		value := gjson.GetBytes(captured, path(description))
		if value.Type != gjson.String || value.Str != want {
			t.Errorf("description %s=%s, want %q; body=%s", path(description), value.Raw, want, captured)
		}
	}
	for field, wantRaw := range fixture.machine {
		if got := gjson.GetBytes(captured, path(field)).Raw; got != wantRaw {
			t.Errorf("machine %s=%s, want raw %s", path(field), got, wantRaw)
		}
	}
	if properties := gjson.GetBytes(captured, path(prefix+".properties")); !strings.HasPrefix(properties.Raw, `{"SECRET_key":`) {
		t.Errorf("raw property name changed: %s", properties.Raw)
	}
	if converted && strings.HasPrefix(fixture.textPath, "generationConfig.responseSchema.") && gjson.GetBytes(captured, "generationConfig.responseSchema").Exists() {
		t.Error("fixed converter left camel responseSchema in wire request")
	}
}

func TestGeminiGenerationConfigNative(t *testing.T) {
	const childKey = "CENSORSHIP_GEMINI_GENERATION_CASE"
	child := os.Getenv(childKey)
	for _, phase := range []string{"BeforeAuth", "AfterAuth"} {
		for _, tc := range geminiGenerationConfigCases() {
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
					cmd := exec.CommandContext(ctx, binary, "-test.run=^TestGeminiGenerationConfigNative$", "-test.v")
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
					for _, fixture := range geminiGenerationConfigFixtures() {
						t.Run(fixture.name, func(t *testing.T) {
							body := functionalHTTPBody(t, fixture, tc.input)
							if !json.Valid(body) {
								t.Fatal("invalid generation fixture")
							}
							headers := http.Header{"Content-Encoding": {"identity"}, "Content-Length": {"123"}, "Transfer-Encoding": {"chunked"}, "X-Preserved": {"SECRET header"}}
							request := pluginapi.RequestInterceptRequest{SourceFormat: "gemini", Model: "target-model", RequestedModel: "requested-decoy", Body: bytes.Clone(body), Headers: headers.Clone(), Metadata: map[string]any{executor.SelectedAuthMetadataKey: "auth-1", "source_model": "source-decoy", "preserved": "SECRET metadata"}}
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
								requireGeminiGenerationConfigWire(t, fixture, response.Body, tc.want, false)
							}
						})
					}
				})
			})
		}
	}
}

func TestGeminiGenerationConfigHTTP(t *testing.T) {
	for _, phase := range []string{"BeforeAuth", "AfterAuth"} {
		for _, tc := range geminiGenerationConfigCases() {
			t.Run(phase+"/"+tc.name, func(t *testing.T) {
				upstream := newFunctionalDirectUpstream(t, "gemini")
				cpa := startFunctionalDirectCPA(t, upstream.URL, "gemini", geminiGenerationConfigYAML(tc, phase, modelName))
				defer func() { t.Logf("Gemini %s/%s CPA log\n%s", phase, tc.name, readCPALog(cpa.logPath)) }()
				fixtures := geminiGenerationConfigFixtures()
				status, _, response := postJSON(t, cpa.baseURL+fixtures[0].endpoint, functionalHTTPBody(t, fixtures[0], "clean canary"))
				if status != 200 || upstream.arrivalCount() != 1 || upstream.requestCount() != 1 {
					t.Fatalf("clean canary status=%d body=%s arrivals=%d requests=%d", status, response, upstream.arrivalCount(), upstream.requestCount())
				}
				for _, fixture := range fixtures {
					t.Run(fixture.name, func(t *testing.T) {
						body := functionalHTTPBody(t, fixture, tc.input)
						original := bytes.Clone(body)
						if !json.Valid(body) {
							t.Fatal("invalid generation fixture")
						}
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
							requireGeminiGenerationConfigWire(t, fixture, upstream.lastRequest(), tc.want, true)
						}
					})
				}
			})
		}
	}
}
