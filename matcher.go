package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type textMatch struct {
	Start int
	End   int
}

func sameFoldRune(a, b rune) bool {
	if a == b {
		return true
	}
	for folded := unicode.SimpleFold(a); folded != a; folded = unicode.SimpleFold(folded) {
		if folded == b {
			return true
		}
	}
	return false
}

func containsRule(text string, rule compiledRule, ignoreCase bool) bool {
	if !ignoreCase {
		return strings.Contains(text, rule.Term)
	}
	if len(rule.Runes) == 0 {
		return false
	}
	for start := range text {
		if _, ok := foldMatchEnd(text, start, rule.Runes); ok {
			return true
		}
	}
	return false
}

func findFoldMatches(text string, runes []rune) []textMatch {
	if len(runes) == 0 {
		return nil
	}
	var matches []textMatch
	for start := 0; start < len(text); {
		if end, ok := foldMatchEnd(text, start, runes); ok {
			matches = append(matches, textMatch{Start: start, End: end})
			start = end
			continue
		}
		_, size := utf8.DecodeRuneInString(text[start:])
		start += size
	}
	return matches
}

func foldMatchEnd(text string, start int, runes []rune) (int, bool) {
	position := start
	for _, want := range runes {
		if position >= len(text) {
			return 0, false
		}
		got, size := utf8.DecodeRuneInString(text[position:])
		if !sameFoldRune(got, want) {
			return 0, false
		}
		position += size
	}
	return position, true
}

func stripRule(text string, rule compiledRule, ignoreCase bool) (string, bool) {
	if !ignoreCase {
		if !strings.Contains(text, rule.Term) {
			return text, false
		}
		return strings.ReplaceAll(text, rule.Term, ""), true
	}
	matches := findFoldMatches(text, rule.Runes)
	if len(matches) == 0 {
		return text, false
	}
	var out strings.Builder
	out.Grow(len(text))
	position := 0
	for _, match := range matches {
		out.WriteString(text[position:match.Start])
		position = match.End
	}
	out.WriteString(text[position:])
	return out.String(), true
}

func obfuscateRule(text string, rule compiledRule, ignoreCase bool, char string) (string, bool) {
	if !ignoreCase {
		if !strings.Contains(text, rule.Term) {
			return text, false
		}
		_, firstSize := utf8.DecodeRuneInString(rule.Term)
		replacement := rule.Term[:firstSize] + char + rule.Term[firstSize:]
		return strings.ReplaceAll(text, rule.Term, replacement), true
	}
	matches := findFoldMatches(text, rule.Runes)
	if len(matches) == 0 {
		return text, false
	}
	var out strings.Builder
	out.Grow(len(text) + len(matches)*len(char))
	position := 0
	for _, match := range matches {
		out.WriteString(text[position:match.Start])
		_, firstSize := utf8.DecodeRuneInString(text[match.Start:match.End])
		out.WriteString(text[match.Start : match.Start+firstSize])
		out.WriteString(char)
		out.WriteString(text[match.Start+firstSize : match.End])
		position = match.End
	}
	out.WriteString(text[position:])
	return out.String(), true
}
