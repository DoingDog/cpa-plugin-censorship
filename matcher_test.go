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

func TestFoldClassRuneUnifiesKelvinSign(t *testing.T) {
	if got, want := foldClassRune('K'), foldClassRune('K'); got != want {
		t.Fatalf("foldClassRune(K) = %U, foldClassRune(K) = %U; want equal keys", got, want)
	}
	if got, want := foldClassRune('K'), foldClassRune('k'); got != want {
		t.Fatalf("foldClassRune(K) = %U, foldClassRune(k) = %U; want equal keys", got, want)
	}
}

func TestFoldMatcherIncludesFailureSuffixRules(t *testing.T) {
	rules := []compiledRule{
		{Term: "bc", Runes: []rune("bc")},
		{Term: "abc", Runes: []rune("abc")},
	}
	matcher := newFoldMatcher(rules)
	if got, ok := matcher.match("ABC"); !ok || got != 0 {
		t.Fatalf("matcher.match() = %d, %t; want suffix rule index 0, true", got, ok)
	}
}

func TestFoldMatcherMatchesRuleMajorOracle(t *testing.T) {
	cases := []struct {
		name  string
		rules []string
		text  string
	}{
		{name: "prefix", rules: []string{"a", "aa"}, text: "cAA"},
		{name: "suffix", rules: []string{"bc", "abc"}, text: "zABC"},
		{name: "duplicate-folded", rules: []string{"K", "k"}, text: "K"},
		{name: "sigma", rules: []string{"Σ", "ς"}, text: "σ"},
		{name: "different-byte-length", rules: []string{"K"}, text: "K"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := make([]compiledRule, len(tc.rules))
			for i, term := range tc.rules {
				rules[i] = compiledRule{Term: term, Runes: []rune(term)}
			}
			want := -1
			for i, rule := range rules {
				if containsRule(tc.text, rule, true) {
					want = i
					break
				}
			}
			got, ok := newFoldMatcher(rules).match(tc.text)
			if got != want || ok != (want >= 0) {
				t.Fatalf("matcher.match() = %d, %t; want oracle %d, %t", got, ok, want, want >= 0)
			}
		})
	}
}

func TestFoldBlockMatcherDoesNotCrossSpans(t *testing.T) {
	rules := []compiledRule{{Term: "abc", Runes: []rune("abc")}}
	cfg := &configSnapshot{
		Mode:         modeBlock,
		IgnoreCase:   true,
		Rules:        rules,
		BlockMatcher: newFoldMatcher(rules),
	}
	spans := []textSpan{
		{Text: "ab", Role: "user"},
		{Text: "c", Role: "developer"},
	}
	blocked, changed := applyMode(spans, cfg)
	if changed || blocked != nil {
		t.Fatalf("applyMode() = %#v, %t; want no block across spans", blocked, changed)
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
	if !matched || got != "a"+char+"A"+"a"+char+"A" {
		t.Fatalf("obfuscateFoldRule() = %q, %t; want %q, true", got, matched, "a"+char+"A"+"a"+char+"A")
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
