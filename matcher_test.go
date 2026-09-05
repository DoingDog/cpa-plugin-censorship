package main

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

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
				if independentFoldContains(tc.text, rule.Term) {
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

func TestFoldKMPResetsAfterNonOverlappingMatch(t *testing.T) {
	rule := forcedFoldedKMPRule("aa")
	got, matched := rewriteFoldedKMP("aaa", rule, "", false)
	if !matched || got != "a" {
		t.Fatalf("rewriteFoldedKMP() = %q, %t; want %q, true", got, matched, "a")
	}
}

func TestFoldKMPPreservesSourceByteSpans(t *testing.T) {
	const char = "⁠"
	invalidTerm := string([]byte{0xff, 'x'})
	invalidText := "a" + string([]byte{0xfe, 'X'}) + "b"
	tests := []struct {
		name      string
		text      string
		term      string
		obfuscate bool
		want      string
	}{
		{name: "sigma strip", text: "xςΣσy", term: "ΣΣ", want: "xσy"},
		{name: "kelvin obfs", text: "aKXb", term: "kx", obfuscate: true, want: "aK" + char + "Xb"},
		{name: "source case obfs", text: "aÉxB", term: "éX", obfuscate: true, want: "aÉ" + char + "xB"},
		{name: "invalid UTF-8 obfs", text: invalidText, term: invalidTerm, obfuscate: true, want: "a" + string([]byte{0xfe}) + char + "Xb"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule := forcedFoldedKMPRule(test.term)
			got, matched := rewriteFoldedKMP(test.text, rule, char, test.obfuscate)
			if !matched || got != test.want {
				t.Fatalf("rewriteFoldedKMP() bytes = % x, %t; want % x, true", got, matched, test.want)
			}
		})
	}
}

func TestFoldedRuleFallsBackWithoutFailureTable(t *testing.T) {
	rule := forcedFoldedKMPRule("aaaab")
	rule.FoldFailure = nil
	prefix := strings.Repeat("a", 4<<10)
	got, matched := stripRule(prefix+"aaaab", rule, true)
	if !matched || got != prefix {
		t.Fatalf("stripRule() = %q, %t; want unchanged prefix, true", got, matched)
	}
}

func TestAdaptiveFoldKMPBoundary(t *testing.T) {
	for _, test := range []struct {
		textBytes, patternScalars int
		want                      bool
	}{
		{textBytes: 1 << 20, patternScalars: 3},
		{textBytes: (4 << 10) - 1, patternScalars: 4},
		{textBytes: 4 << 10, patternScalars: 4, want: true},
		{textBytes: 64 << 10, patternScalars: 16, want: true},
	} {
		if got := useFoldedKMP(test.textBytes, test.patternScalars); got != test.want {
			t.Errorf("useFoldedKMP(%d, %d) = %t, want %t", test.textBytes, test.patternScalars, got, test.want)
		}
	}
}

func independentFoldContains(text, term string) bool {
	termRunes := []rune(term)
	if len(termRunes) == 0 {
		return true
	}
	for start := range text {
		window := make([]rune, 0, len(termRunes))
		for _, r := range text[start:] {
			if len(window) == len(termRunes) {
				break
			}
			window = append(window, r)
		}
		if len(window) != len(termRunes) {
			continue
		}
		matched := true
		for i, r := range window {
			if !independentFoldEqual(r, termRunes[i]) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func independentFoldEqual(got, want rune) bool {
	for candidate := got; ; candidate = unicode.SimpleFold(candidate) {
		if candidate == want {
			return true
		}
		if unicode.SimpleFold(candidate) == got {
			return false
		}
	}
}

func independentFoldClassRune(r rune) rune {
	if r < utf8.RuneSelf {
		return unicode.ToLower(r)
	}
	class := r
	for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
		if folded >= 'a' && folded <= 'z' {
			return folded
		}
		if folded < class {
			class = folded
		}
	}
	return class
}

func forcedFoldedKMPRule(term string) compiledRule {
	rule := compiledRule{Term: term}
	for _, r := range term {
		rule.Runes = append(rule.Runes, independentFoldClassRune(r))
	}
	rule.FoldFailure = make([]int, len(rule.Runes))
	for i, prefix := 1, 0; i < len(rule.Runes); i++ {
		for prefix > 0 && rule.Runes[i] != rule.Runes[prefix] {
			prefix = rule.FoldFailure[prefix-1]
		}
		if rule.Runes[i] == rule.Runes[prefix] {
			prefix++
		}
		rule.FoldFailure[i] = prefix
	}
	return rule
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

func TestFoldMatcherASCIITransitionsSurviveClearedRootMap(t *testing.T) {
	cfg := &configSnapshot{
		Mode:       modeBlock,
		IgnoreCase: true,
		Rules:      []compiledRule{{Term: "ab"}},
	}
	if err := compileSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	matcher := cfg.BlockMatcher
	var root [utf8.RuneSelf]int = matcher.asciiRoot
	if root['a'] == 0 {
		t.Fatal("ASCII root table has no transition for a")
	}
	matcher.nodes[0].next = nil
	if got, ok := matcher.match("xxAB"); !ok || got != 0 {
		t.Fatalf("matcher.match() = %d, %t; want 0, true", got, ok)
	}
}

func TestFoldMatcherFailureToRootUsesASCIITable(t *testing.T) {
	cfg := &configSnapshot{
		Mode:       modeBlock,
		IgnoreCase: true,
		Rules: []compiledRule{
			{Term: "acx"},
			{Term: "b"},
		},
	}
	if err := compileSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	matcher := cfg.BlockMatcher
	matcher.nodes[0].next = nil
	if got, ok := matcher.match("acB"); !ok || got != 1 {
		t.Fatalf("matcher.match() after failure to root = %d, %t; want 1, true", got, ok)
	}
}

var (
	exactRewriteTextSink  string
	exactRewriteMatchSink bool
)

func TestRewriteExactReportsMatchesByLength(t *testing.T) {
	obfs := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: "éx"}},
		ObfsChar: "​",
	}
	if err := compileSnapshot(obfs); err != nil {
		t.Fatal(err)
	}
	invalidTerm := string([]byte{0xff, 'x'})
	invalid := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: invalidTerm}},
		ObfsChar: "​",
	}
	if err := compileSnapshot(invalid); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		text      string
		rule      compiledRule
		obfuscate bool
		want      string
		matched   bool
	}{
		{name: "strip hit", text: "aaaaa", rule: compiledRule{Term: "aa"}, want: "a", matched: true},
		{name: "strip miss", text: "plain", rule: compiledRule{Term: "aa"}, want: "plain"},
		{name: "obfs multibyte", text: "éxéx", rule: obfs.Rules[0], obfuscate: true, want: "é​xé​x", matched: true},
		{name: "obfs miss", text: "plain", rule: obfs.Rules[0], obfuscate: true, want: "plain"},
		{name: "obfs invalid UTF-8", text: "a" + invalidTerm, rule: invalid.Rules[0], obfuscate: true, want: "a" + string([]byte{0xff}) + "​x", matched: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, matched := rewriteExact(test.text, test.rule, test.obfuscate)
			if got != test.want || matched != test.matched {
				t.Fatalf("rewriteExact() = %q, %t; want %q, %t", got, matched, test.want, test.matched)
			}
		})
	}
}

func TestExactRewriteMissAllocatesNothing(t *testing.T) {
	obfs := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: "secret"}},
		ObfsChar: "​",
	}
	if err := compileSnapshot(obfs); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		rule      compiledRule
		obfuscate bool
	}{
		{name: "strip", rule: compiledRule{Term: "secret"}},
		{name: "obfs", rule: obfs.Rules[0], obfuscate: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(1000, func() {
				exactRewriteTextSink, exactRewriteMatchSink = rewriteExact("plain text", test.rule, test.obfuscate)
			})
			if allocs != 0 {
				t.Fatalf("rewriteExact() miss allocations = %.1f, want 0", allocs)
			}
		})
	}
}

func TestExactObfuscationHitAllocationCeiling(t *testing.T) {
	cfg := &configSnapshot{
		Mode:     modeObfs,
		Rules:    []compiledRule{{Term: "éx"}},
		ObfsChar: "⁠",
	}
	if err := compileSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("éx", 256)
	allocs := testing.AllocsPerRun(100, func() {
		exactRewriteTextSink, exactRewriteMatchSink = rewriteExact(text, cfg.Rules[0], true)
	})
	if allocs > 1 {
		t.Fatalf("rewriteExact() hit allocations = %.1f, want <= 1", allocs)
	}
	if !exactRewriteMatchSink || len(exactRewriteTextSink) != len(text)+256*len(cfg.ObfsChar) {
		t.Fatalf("rewriteExact() hit = len %d, %t", len(exactRewriteTextSink), exactRewriteMatchSink)
	}
}

func TestByteMatcherPreservesExactBlockOrder(t *testing.T) {
	tests := []struct {
		name  string
		rules []string
		text  string
		want  int
		ok    bool
	}{
		{name: "lowest YAML index despite later occurrence", rules: []string{"later", "first"}, text: "first then later", want: 0, ok: true},
		{name: "failure suffix", rules: []string{"bc", "abc"}, text: "abc", want: 0, ok: true},
		{name: "prefix", rules: []string{"ab", "abc"}, text: "abc", want: 0, ok: true},
		{name: "duplicate rules", rules: []string{"hit", "hit"}, text: "hit", want: 0, ok: true},
		{name: "overlap", rules: []string{"aa", "aaa"}, text: "aaa", want: 0, ok: true},
		{name: "NUL", rules: []string{"\x00b", "other"}, text: "a\x00b", want: 0, ok: true},
		{name: "no match", rules: []string{"abc", "def"}, text: "plain", want: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rules := make([]compiledRule, len(test.rules))
			for i, term := range test.rules {
				rules[i] = compiledRule{Term: term}
			}
			got, ok := newByteMatcher(rules, 0).match(test.text)
			if got != test.want || ok != test.ok {
				t.Fatalf("match() = %d, %t; want %d, %t", got, ok, test.want, test.ok)
			}
		})
	}

	rules := []compiledRule{{Term: "prefix"}, {Term: "tail"}, {Term: "last"}}
	if got, ok := newByteMatcher(rules[1:], 1).match("last tail"); !ok || got != 1 {
		t.Fatalf("offset matcher = %d, %t; want 1, true", got, ok)
	}
}

func TestByteMatcherDoesNotCrossSpans(t *testing.T) {
	matcher := newByteMatcher([]compiledRule{{Term: "abc"}}, 0)
	for _, text := range []string{"ab", "c"} {
		if got, ok := matcher.match(text); ok || got != -1 {
			t.Fatalf("match(%q) = %d, %t; want -1, false", text, got, ok)
		}
	}
}

func TestByteMatcherMatchesInvalidUTF8ByByte(t *testing.T) {
	term := string([]byte{0xff, 0x00, 'x'})
	text := "prefix" + term + string([]byte{0xfe})
	matcher := newByteMatcher([]compiledRule{{Term: term}}, 0)
	if got, ok := matcher.match(text); !ok || got != 0 {
		t.Fatalf("invalid UTF-8 match = %d, %t; want 0, true", got, ok)
	}
	if got, ok := matcher.match(string([]byte{0xff, 0x00, 'y'})); ok || got != -1 {
		t.Fatalf("invalid UTF-8 near miss = %d, %t; want -1, false", got, ok)
	}
}

func TestAdaptiveExactBlockBoundary(t *testing.T) {
	for _, test := range []struct {
		rules, textBytes int
		want             bool
	}{
		{rules: 255, textBytes: 1 << 20},
		{rules: 256, textBytes: (16 << 10) - 1},
		{rules: 256, textBytes: 16 << 10, want: true},
		{rules: 512, textBytes: 64 << 10, want: true},
		{rules: 128, textBytes: 64 << 10},
	} {
		if got := useExactByteMatcher(test.rules, test.textBytes); got != test.want {
			t.Errorf("useExactByteMatcher(%d, %d) = %t, want %t", test.rules, test.textBytes, got, test.want)
		}
	}

	rules := make([]compiledRule, 256)
	for i := range rules {
		rules[i] = compiledRule{Term: "term-" + string(rune('A'+i))}
	}
	exactBlock := &configSnapshot{Mode: modeBlock, Rules: append([]compiledRule(nil), rules...)}
	if err := compileSnapshot(exactBlock); err != nil {
		t.Fatal(err)
	}
	if exactBlock.ExactBlockMatcher == nil {
		t.Fatal("exact block snapshot left ExactBlockMatcher nil")
	}

	belowBoundary := &configSnapshot{Mode: modeBlock, Rules: append([]compiledRule(nil), rules[:255]...)}
	if err := compileSnapshot(belowBoundary); err != nil {
		t.Fatal(err)
	}
	if belowBoundary.ExactBlockMatcher != nil {
		t.Fatal("exact block snapshot below rule boundary compiled ExactBlockMatcher")
	}

	exactStrip := &configSnapshot{Mode: modeStrip, Rules: append([]compiledRule(nil), rules...)}
	if err := compileSnapshot(exactStrip); err != nil {
		t.Fatal(err)
	}
	if exactStrip.ExactBlockMatcher != nil {
		t.Fatal("exact strip snapshot compiled ExactBlockMatcher")
	}

	foldedBlock := &configSnapshot{Mode: modeBlock, IgnoreCase: true, Rules: append([]compiledRule(nil), rules...)}
	if err := compileSnapshot(foldedBlock); err != nil {
		t.Fatal(err)
	}
	if foldedBlock.ExactBlockMatcher != nil {
		t.Fatal("folded block snapshot compiled ExactBlockMatcher")
	}
}

func TestAdaptiveExactBlockPreservesRuleAndDocumentOrder(t *testing.T) {
	rules := make([]compiledRule, 256)
	for i := range rules {
		rules[i] = compiledRule{Term: "term-" + string(rune('A'+i))}
	}
	rules[1].Term = "prefix-hit"
	rules[4].Term = "tail-low"
	rules[255].Term = "tail-high"
	cfg := &configSnapshot{Mode: modeBlock, Rules: rules}
	if err := compileSnapshot(cfg); err != nil {
		t.Fatal(err)
	}
	padding := strings.Repeat("x", 16<<10)

	tests := []struct {
		name     string
		spans    []textSpan
		wantTerm string
		wantRole string
	}{
		{
			name: "prefix rule beats earlier tail occurrence",
			spans: []textSpan{
				{Text: padding + " tail-high", Role: "user"},
				{Text: "prefix-hit", Role: "developer"},
			},
			wantTerm: "prefix-hit",
			wantRole: "developer",
		},
		{
			name: "lower tail rule beats earlier span",
			spans: []textSpan{
				{Text: padding + " tail-high", Role: "user"},
				{Text: "tail-low", Role: "developer"},
			},
			wantTerm: "tail-low",
			wantRole: "developer",
		},
		{
			name: "same tail rule keeps document order",
			spans: []textSpan{
				{Text: padding + " tail-low", Role: "user"},
				{Text: "tail-low", Role: "developer"},
			},
			wantTerm: "tail-low",
			wantRole: "user",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blocked, changed := applyMode(test.spans, cfg)
			if changed || blocked == nil || blocked.Term != test.wantTerm || blocked.Role != test.wantRole {
				t.Fatalf("applyMode() = %#v, %t; want %q, %q", blocked, changed, test.wantTerm, test.wantRole)
			}
		})
	}
}
