package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

var selectedTextFormats = []struct {
	format, prefix, suffix, path string
}{
	{"openai", `{"messages":[{"role":"user","content":`, `}]}`, "messages.0.content"},
	{"openai-response", `{"input":`, `}`, "input"},
	{"claude", `{"messages":[{"role":"user","content":`, `}]}`, "messages.0.content"},
	{"gemini", `{"contents":[{"role":"user","parts":[{"text":`, `}]}]}`, "contents.0.parts.0.text"},
	{"interactions", `{"input":`, `}`, "input"},
}

func TestSelectedTextUsesStandardJSONDecoding(t *testing.T) {
	for _, raw := range []string{
		`"\ud800\u0065xample note"`,
		`"\udc00\u0065xample note"`,
		`"\ud83d\ude00example note"`,
		`"\ud800\u0065\u0078\u0061mple note"`,
		`"中文😀example note"`,
		`"\ud800\u0065xample\n\"note\""`,
		`""`,
	} {
		var want string
		if err := json.Unmarshal([]byte(raw), &want); err != nil {
			t.Fatal(err)
		}
		for _, f := range selectedTextFormats {
			t.Run(f.format+"/"+raw, func(t *testing.T) {
				body := []byte(f.prefix + raw + f.suffix)
				spans, err := selectTextSpans(body, f.format, scopeSet{"user": {}})
				if err != nil {
					t.Fatal(err)
				}
				if want == "" {
					if len(spans) != 0 {
						t.Fatalf("empty token selected: %#v", spans)
					}
					return
				}
				if len(spans) != 1 || spans[0].Text != want || spans[0].Role != "user" {
					t.Fatalf("spans=%#v want text=%q role=user", spans, want)
				}
				if !bytes.Equal(body[spans[0].RawStart:spans[0].RawEnd], []byte(raw)) {
					t.Fatalf("selected range does not contain original token: %#v", spans[0])
				}
			})
		}
	}
}

func TestSelectedTextDecodedRules(t *testing.T) {
	for _, raw := range []string{`"\ud800\u0065xample note"`, `"\udc00\u0065xample note"`, `"\ud83d\ude00example note"`, `"中文\n\"example\" note"`} {
		var original string
		if err := json.Unmarshal([]byte(raw), &original); err != nil {
			t.Fatal(err)
		}
		for _, f := range selectedTextFormats {
			for _, fold := range []bool{false, true} {
				for _, action := range []string{"block", "strip", "obfs", "strip-other"} {
					t.Run(fmt.Sprintf("%s/%s/fold=%t/%s", f.format, raw, fold, action), func(t *testing.T) {
						term, phase := "example", action
						if action == "strip-other" {
							term, phase = "note", "strip"
						}
						if fold {
							term = strings.ToUpper(term)
						}
						cfg := mustConfig(t, fmt.Sprintf("words:\n  %s: [%s]\nignore_case: %t\n", phase, term, fold))
						result, err := transformRequest([]byte(f.prefix+raw+f.suffix), f.format, cfg)
						if err != nil || result.Invalid {
							t.Fatalf("result=%#v err=%v", result, err)
						}
						if action == "block" {
							if result.Blocked == nil || *result.Blocked != (blockMatch{Term: term, Role: "user"}) || result.Body != nil {
								t.Fatalf("result=%#v want example block", result)
							}
							return
						}
						want := strings.ReplaceAll(original, "example", "")
						if action == "obfs" {
							want = strings.ReplaceAll(original, "example", "e​xample")
						} else if action == "strip-other" {
							want = strings.ReplaceAll(original, "note", "")
						}
						var got string
						if err := json.Unmarshal([]byte(gjson.GetBytes(result.Body, f.path).Raw), &got); err != nil {
							t.Fatalf("body=%q err=%v", result.Body, err)
						}
						if result.Blocked != nil || got != want {
							t.Fatalf("result=%#v text=%q want=%q", result, got, want)
						}
					})
				}
			}
		}
	}
}

func TestSelectedTextDormantDescriptionDecoding(t *testing.T) {
	const raw = `"\ud800\u0065xample note"`
	var original string
	if err := json.Unmarshal([]byte(raw), &original); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"block", "strip", "obfs", "dormant"} {
		t.Run(action, func(t *testing.T) {
			phase := action
			if action == "dormant" {
				phase = "block"
			}
			config := "words:\n  strip: [FLAT]\n"
			if phase == "strip" {
				config = "words:\n  strip: [FLAT, example]\n"
			} else {
				config += "  " + phase + ": [example]\n"
			}
			flat := "FLAT"
			if action == "dormant" {
				flat = "keep"
			}
			body := []byte(`{"tools":[{"type":"function","name":"f","description":"` + flat + `","function":{"description":` + raw + `}}]}`)
			result, err := transformRequest(body, "openai-response", mustConfig(t, config))
			if err != nil || result.Invalid {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if action == "dormant" {
				if result.Body != nil || result.Blocked != nil {
					t.Fatalf("dormant token activated: %#v", result)
				}
				return
			}
			if action == "block" {
				if result.Blocked == nil || result.Blocked.Term != "example" || result.Blocked.Role != "developer" || result.Body != nil {
					t.Fatalf("result=%#v want activated block", result)
				}
				return
			}
			want := strings.ReplaceAll(original, "example", "")
			if action == "obfs" {
				want = strings.ReplaceAll(original, "example", "e​xample")
			}
			var got string
			if err := json.Unmarshal([]byte(gjson.GetBytes(result.Body, "tools.0.function.description").Raw), &got); err != nil {
				t.Fatal(err)
			}
			if got != want || gjson.GetBytes(result.Body, "tools.0.description").Str != "" {
				t.Fatalf("body=%q got=%q want=%q", result.Body, got, want)
			}
		})
	}
}

func TestSelectedTextNoOpPreservesBytes(t *testing.T) {
	for _, action := range []string{"block", "strip", "obfs"} {
		cfg := mustConfig(t, "words:\n  "+action+": [MISS]\n")
		for _, f := range selectedTextFormats {
			body := []byte(" \n" + f.prefix + `"\ud800\u0065xample😀"` + f.suffix + " \t")
			before := append([]byte(nil), body...)
			result, err := transformRequest(body, f.format, cfg)
			if err != nil || result.Invalid || result.Blocked != nil || result.Body != nil || !bytes.Equal(body, before) {
				t.Fatalf("format=%s action=%s result=%#v err=%v", f.format, action, result, err)
			}
		}
	}
	body := []byte(` { "input":"CHANGE", "n":1e+03, "unselected":"\ud800\u0065xample", "pair":"😀", "tools":[{"type":"function","name":"f","description":"keep","function":{"description":"\ud800\u0065xample"}}] } `)
	want := bytes.Replace(body, []byte(`"CHANGE"`), []byte(`""`), 1)
	result, err := transformRequest(body, "openai-response", mustConfig(t, "words:\n  strip: [CHANGE]\n"))
	if err != nil || result.Invalid || result.Blocked != nil || !bytes.Equal(result.Body, want) {
		t.Fatalf("body=%q want=%q err=%v", result.Body, want, err)
	}
}
