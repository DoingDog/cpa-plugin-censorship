package main

import "testing"

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
