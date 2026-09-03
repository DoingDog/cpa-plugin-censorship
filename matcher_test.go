package main

import "testing"

func TestFoldMatcherSelectsLowestRuleIndexAcrossOccurrences(t *testing.T) {
	rules := []compiledRule{
		{Term: "ab", Runes: []rune("ab")},
		{Term: "a", Runes: []rune("a")},
	}
	matcher := newFoldMatcher(rules)
	if got, ok := matcher.match("xABy"); !ok || got != 0 {
		t.Fatalf("matcher.match() = %d, %t; want rule index 0, true", got, ok)
	}
}

func TestFoldBlockMatcherPreservesRuleMajorDocumentRole(t *testing.T) {
	rules := []compiledRule{
		{Term: "ab", Runes: []rune("ab")},
		{Term: "a", Runes: []rune("a")},
	}
	cfg := &configSnapshot{
		Mode:         modeBlock,
		IgnoreCase:   true,
		Rules:        rules,
		BlockMatcher: newFoldMatcher(rules),
	}
	spans := []textSpan{
		{Text: "a", Role: "user"},
		{Text: "AB", Role: "developer"},
	}
	blocked, changed := applyMode(spans, cfg)
	if changed || blocked == nil {
		t.Fatalf("applyMode() = %#v, %t; want block", blocked, changed)
	}
	if blocked.Term != "ab" || blocked.Role != "developer" {
		t.Fatalf("block = %#v; want term ab and role developer", blocked)
	}
}

func TestFoldStripRuleProcessesAllNonOverlappingOccurrences(t *testing.T) {
	got, matched := stripFoldRule("aAaAA", []rune("aa"))
	if !matched || got != "A" {
		t.Fatalf("stripFoldRule() = %q, %t; want %q, true", got, matched, "A")
	}
}

func TestFoldObfuscateRulePreservesSourceCasePerOccurrence(t *testing.T) {
	const char = "⁠"
	got, matched := obfuscateFoldRule("aAaA", []rune("aa"), char)
	if !matched || got != "a"+char+"Aa"+char+"A" {
		t.Fatalf("obfuscateFoldRule() = %q, %t; want %q, true", got, matched, "a"+char+"Aa"+char+"A")
	}
}

func TestContainsRuleCaseModes(t *testing.T) {
	rule := compiledRule{Term: "Alpha", Runes: []rune("Alpha")}
	if containsRule("alpha", rule, false) {
		t.Fatal("default matching ignored case")
	}
	if !containsRule("xxaLPHAyy", rule, true) {
		t.Fatal("ignore_case did not match ASCII mixed case")
	}
	cases := []struct {
		text string
		term string
		want bool
	}{
		{text: "ς", term: "Σ", want: true},
		{text: "K", term: "K", want: true},
		{text: "STRASSE", term: "straße", want: false},
		{text: "é", term: "é", want: false},
	}
	for _, tc := range cases {
		r := compiledRule{Term: tc.term, Runes: []rune(tc.term)}
		if got := containsRule(tc.text, r, true); got != tc.want {
			t.Errorf("containsRule(%q, %q) = %t, want %t", tc.text, tc.term, got, tc.want)
		}
	}
}
