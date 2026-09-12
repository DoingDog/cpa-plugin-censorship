package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestSelectorHasEnabledRoleByFormat(t *testing.T) {
	canonicalRoles := []string{"system", "developer", "user", "assistant", "tool"}
	formats := []struct {
		name    string
		enabled scopeSet
	}{
		{name: "openai", enabled: scopeSet{"system": struct{}{}, "developer": struct{}{}, "user": struct{}{}, "assistant": struct{}{}, "tool": struct{}{}}},
		{name: "openai-response", enabled: scopeSet{"system": struct{}{}, "developer": struct{}{}, "user": struct{}{}, "assistant": struct{}{}, "tool": struct{}{}}},
		{name: "claude", enabled: scopeSet{"system": struct{}{}, "user": struct{}{}, "assistant": struct{}{}, "tool": struct{}{}}},
		{name: "gemini", enabled: scopeSet{"system": struct{}{}, "user": struct{}{}, "assistant": struct{}{}}},
		{name: "interactions", enabled: scopeSet{"system": struct{}{}, "user": struct{}{}, "assistant": struct{}{}}},
		{name: "unknown", enabled: scopeSet{}},
	}

	for _, format := range formats {
		for _, role := range canonicalRoles {
			roles := scopeSet{role: struct{}{}}
			_, want := format.enabled[role]
			if got := selectorHasEnabledRole(format.name, roles); got != want {
				t.Fatalf("selectorHasEnabledRole(%q, %q) = %t, want %t", format.name, role, got, want)
			}
		}
		if selectorHasEnabledRole(format.name, scopeSet{}) {
			t.Fatalf("selectorHasEnabledRole(%q, empty) = true, want false", format.name)
		}
	}
}

func TestOpenAIRoleGateUsesCanonicalToolRole(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		sourceFormat string
		roles        scopeSet
		want         []struct{ text, role string }
	}{
		{
			name:         "function message enabled",
			body:         `{"messages":[{"role":"function","content":"function content"}]}`,
			sourceFormat: "openai",
			roles:        scopeSet{"tool": {}},
			want:         []struct{ text, role string }{{text: "function content", role: "tool"}},
		},
		{
			name:         "function message disabled",
			body:         `{"messages":[{"role":"function","content":"function content"}]}`,
			sourceFormat: "openai",
			roles:        scopeSet{"user": {}},
		},
		{
			name:         "function output enabled",
			body:         `{"input":[{"type":"function_call_output","role":"user","output":"function output"}]}`,
			sourceFormat: "openai-response",
			roles:        scopeSet{"tool": {}},
			want:         []struct{ text, role string }{{text: "function output", role: "tool"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := selectTextSpans([]byte(tc.body), tc.sourceFormat, tc.roles)
			if err != nil {
				t.Fatalf("selectTextSpans(%q) error = %v", tc.sourceFormat, err)
			}
			if len(spans) != len(tc.want) {
				t.Fatalf("selectTextSpans(%q) spans = %#v, want %d spans", tc.sourceFormat, spans, len(tc.want))
			}
			for i, want := range tc.want {
				if spans[i].Text != want.text || spans[i].Role != want.role {
					t.Fatalf("span[%d] = {Text:%q Role:%q}, want {Text:%q Role:%q}", i, spans[i].Text, spans[i].Role, want.text, want.role)
				}
			}
		})
	}
}

func TestSelectorRoleGatesPreserveCanonicalOverrides(t *testing.T) {
	assertSpans := func(body, sourceFormat string, roles scopeSet, want []struct{ text, role string }) {
		t.Helper()
		spans, err := selectTextSpans([]byte(body), sourceFormat, roles)
		if err != nil {
			t.Fatalf("selectTextSpans(%q) error = %v", sourceFormat, err)
		}
		if len(spans) != len(want) {
			t.Fatalf("selectTextSpans(%q) spans = %#v, want %d spans", sourceFormat, spans, len(want))
		}
		for i, expected := range want {
			if spans[i].Text != expected.text || spans[i].Role != expected.role {
				t.Fatalf("span[%d] = {Text:%q Role:%q}, want {Text:%q Role:%q}", i, spans[i].Text, spans[i].Role, expected.text, expected.role)
			}
		}
	}

	t.Run("openai responses output types use assistant", func(t *testing.T) {
		assertSpans(`{"input":[{"type":"message","role":"user","content":[{"type":"output_text","text":"enabled output"}]},{"type":"message","role":"user","content":[{"type":"refusal","refusal":"enabled refusal"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"disabled input"}]}]}`,
			"openai-response", scopeSet{"assistant": struct{}{}}, []struct{ text, role string }{
				{text: "enabled output", role: "assistant"},
				{text: "enabled refusal", role: "assistant"},
			})
	})

	t.Run("claude tool result overrides user message", func(t *testing.T) {
		assertSpans(`{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"enabled tool"}]}]}]}`,
			"claude", scopeSet{"tool": struct{}{}}, []struct{ text, role string }{
				{text: "enabled tool", role: "tool"},
			})
	})

	t.Run("gemini advances roles through disabled and invalid entries", func(t *testing.T) {
		assertSpans(`{"contents":[{"role":"user","parts":[{"text":"disabled user"}]},{"parts":[{"text":"enabled assistant"}]},{"role":null,"parts":[{"text":"invalid role"}]},{"parts":[{"text":"enabled assistant after invalid"}]}]}`,
			"gemini", scopeSet{"assistant": struct{}{}}, []struct{ text, role string }{
				{text: "enabled assistant", role: "assistant"},
				{text: "enabled assistant after invalid", role: "assistant"},
			})
	})

	t.Run("interactions descends from disabled parent to assistant children", func(t *testing.T) {
		assertSpans(`{"input":[{"role":"user","steps":[{"content":"disabled nested user"},{"role":"assistant","parts":[{"type":"","text":"enabled nested assistant"}]},{"type":"model_output","content":{"type":"text","text":"enabled model output"}}]}]}`,
			"interactions", scopeSet{"assistant": struct{}{}}, []struct{ text, role string }{
				{text: "enabled nested assistant", role: "assistant"},
				{text: "enabled model output", role: "assistant"},
			})
	})

	t.Run("interactions user input preserves explicit assistant role", func(t *testing.T) {
		assertSpans(`{"input":[{"role":"assistant","type":"user_input","content":"enabled assistant"}]}`,
			"interactions", scopeSet{"assistant": struct{}{}}, []struct{ text, role string }{
				{text: "enabled assistant", role: "assistant"},
			})
	})
}

func TestSelectorRoleGateRunsAfterGlobalValidation(t *testing.T) {
	invalidBodies := []struct {
		name string
		body []byte
	}{
		{name: "empty", body: []byte(" \n\t ")},
		{name: "malformed", body: []byte(`{"`)},
		{name: "non-object", body: []byte(`[]`)},
		{name: "duplicate member", body: []byte(`{"input":[],"input":[]}`)},
	}
	formats := []string{"openai", "openai-response", "claude", "gemini", "interactions"}
	roles := scopeSet{"disabled": struct{}{}}

	for _, sourceFormat := range formats {
		for _, tc := range invalidBodies {
			t.Run(sourceFormat+"/"+tc.name, func(t *testing.T) {
				spans, err := selectTextSpans(tc.body, sourceFormat, roles)
				if err != errInvalidRequest {
					t.Fatalf("selectTextSpans(%q, %q) error = %v, want %v", sourceFormat, tc.name, err, errInvalidRequest)
				}
				if spans != nil {
					t.Fatalf("selectTextSpans(%q, %q) spans = %#v, want nil", sourceFormat, tc.name, spans)
				}
			})
		}
	}
}

func nestedObjectBody(depth int) []byte {
	body := []byte(`0`)
	for i := 0; i < depth; i++ {
		body = append([]byte(`{"a":`), append(body, '}')...)
	}
	return body
}

func TestSelectorRejectsJSONBeyondConfiguredNestingDepth(t *testing.T) {
	roles := scopeSet{"user": {}}
	if _, err := selectTextSpans(nestedObjectBody(1024), "openai", roles); err != nil {
		t.Fatalf("depth 1024 error = %v, want nil", err)
	}
	if _, err := selectTextSpans(nestedObjectBody(1025), "openai", roles); err != errInvalidRequest {
		t.Fatalf("depth 1025 error = %v, want %v", err, errInvalidRequest)
	}
}

func TestJSONNestingScanIgnoresBracketsInsideStrings(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"{ [ ] } \"quoted\""}]}`)
	if _, err := selectTextSpans(body, "openai", scopeSet{"user": {}}); err != nil {
		t.Fatalf("string brackets caused error = %v", err)
	}
}

func TestDisabledSelectorRoleTraversalAllocationCeiling(t *testing.T) {
	const itemCount = 128
	escapedText := `"\u0053\u0045\u0043\u0052\u0045\u0054"`
	repeatItems := func(item string) string {
		return strings.TrimSuffix(strings.Repeat(item+",", itemCount), ",")
	}

	openAI := `{"messages":[` + repeatItems(`{"role":"user","content":`+escapedText+`}`) + `]}`
	openAIResponses := `{"input":[` + repeatItems(`{"type":"message","role":"user","content":[{"type":"input_text","text":`+escapedText+`}]}`) + `]}`
	claude := `{"messages":[` + repeatItems(`{"role":"user","content":[{"type":"text","text":`+escapedText+`}]}`) + `]}`
	gemini := `{"contents":[` + repeatItems(`{"role":"user","parts":[{"text":`+escapedText+`}]}`) + `]}`
	interactions := `{"input":[` + repeatItems(`{"role":"user","content":`+escapedText+`}`) + `]}`
	roles := scopeSet{"disabled": struct{}{}}

	tests := []struct {
		name    string
		root    gjson.Result
		collect func(gjson.Result, scopeSet, *[]textSpan)
	}{
		{name: "openai", root: gjson.Parse(openAI), collect: collectOpenAI},
		{name: "openai response", root: gjson.Parse(openAIResponses), collect: collectOpenAIResponses},
		{name: "claude", root: gjson.Parse(claude), collect: collectClaude},
		{name: "gemini", root: gjson.Parse(gemini), collect: collectGemini},
		{name: "interactions", root: gjson.Parse(interactions), collect: collectInteractions},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spans := make([]textSpan, 0, itemCount)
			allocations := testing.AllocsPerRun(100, func() {
				spans = spans[:0]
				tc.collect(tc.root, roles, &spans)
			})
			if allocations > 32 {
				t.Fatalf("allocations = %.1f, want at most 32", allocations)
			}
		})
	}
}

func TestSelectorRejectsDeepJSONBeforeGJSONValidation(t *testing.T) {
	const childEnv = "SELECTORS_DEEP_JSON_CHILD"
	if os.Getenv(childEnv) == "1" {
		const depth = maxJSONNestingDepth * 1024
		body := []byte(strings.Repeat(`{"a":`, depth) + `0` + strings.Repeat(`}`, depth))
		if _, err := selectTextSpans(body, "openai", scopeSet{"user": {}}); err != errInvalidRequest {
			t.Fatalf("depth %d error = %v, want %v", depth, err, errInvalidRequest)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSelectorRejectsDeepJSONBeforeGJSONValidation$")
	cmd.Env = append(os.Environ(), childEnv+"=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("deep JSON subprocess did not finish promptly: %s", output)
	}
	if err != nil {
		t.Fatalf("deep JSON subprocess failed: %v\n%s", err, output)
	}
}

func TestSelectorRejectsInvalidUTF8(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{name: "key", body: []byte("{\"\xff\":\"safe\"}")},
		{name: "selected value", body: []byte("{\"messages\":[{\"role\":\"user\",\"content\":\"\xff\"}]}")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spans, err := selectTextSpans(tc.body, "openai", scopeSet{"user": {}})
			if err != errInvalidRequest {
				t.Fatalf("error = %v, want %v", err, errInvalidRequest)
			}
			if spans != nil {
				t.Fatalf("spans = %#v, want nil", spans)
			}
		})
	}
}

func TestSelectorSkipsEmptyStringSpans(t *testing.T) {
	const itemCount = 2048
	emptyMessage := `{"role":"user","content":""}`
	body := []byte(`{"messages":[` + strings.TrimSuffix(strings.Repeat(emptyMessage+",", itemCount), ",") + `]}`)

	spans, err := selectTextSpans(body, "openai", scopeSet{"user": {}})
	if err != nil {
		t.Fatalf("empty strings error = %v", err)
	}
	if len(spans) != 0 {
		t.Fatalf("empty strings produced %d spans, want 0", len(spans))
	}

	escapedBody := `{"messages":[{"role":"user","content":"` + string([]byte{92}) + `u0058"}]}`
	spans, err = selectTextSpans([]byte(escapedBody), "openai", scopeSet{"user": {}})
	if err != nil {
		t.Fatalf("escaped string error = %v", err)
	}
	if len(spans) != 1 || spans[0].Text != "X" {
		t.Fatalf("escaped string spans = %#v, want one X span", spans)
	}
}

func TestScanTextPartIgnoresNullMachineDiscriminators(t *testing.T) {
	for _, body := range []string{
		`{"functionCall":null,"text":"allowed"}`,
		`{"text":"allowed","functionCall":null}`,
		`{"function_call":null,"text":"allowed"}`,
		`{"text":"allowed","function_call":null}`,
	} {
		text, allowed, _ := scanTextPart(gjson.Parse(body), false)
		if !allowed || text.Str != "allowed" {
			t.Fatalf("scanTextPart(%s) = (%q, %t), want (allowed, true)", body, text.Str, allowed)
		}
	}

	for _, body := range []string{
		`{"functionCall":{},"text":"excluded"}`,
		`{"function_call":false,"text":"excluded"}`,
		`{"thought":true,"text":"excluded"}`,
	} {
		if _, allowed, _ := scanTextPart(gjson.Parse(body), false); allowed {
			t.Fatalf("scanTextPart(%s) allowed = true, want false", body)
		}
	}
}
