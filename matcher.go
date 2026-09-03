package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

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

func foldClassRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	if r < utf8.RuneSelf {
		return r
	}
	minimum := r
	for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
		if folded < minimum {
			minimum = folded
		}
	}
	if minimum >= 'A' && minimum <= 'Z' {
		return minimum + ('a' - 'A')
	}
	return minimum
}

type foldMatcherNode struct {
	next    map[rune]int
	fail    int
	minRule int
}

type foldMatcher struct {
	nodes []foldMatcherNode
}

func newFoldMatcher(rules []compiledRule) *foldMatcher {
	matcher := &foldMatcher{nodes: []foldMatcherNode{{minRule: -1}}}
	for ruleIndex, rule := range rules {
		if len(rule.Runes) == 0 {
			continue
		}
		nodeIndex := 0
		for _, r := range rule.Runes {
			key := foldClassRune(r)
			next, ok := matcher.nodes[nodeIndex].next[key]
			if !ok {
				if matcher.nodes[nodeIndex].next == nil {
					matcher.nodes[nodeIndex].next = make(map[rune]int)
				}
				next = len(matcher.nodes)
				matcher.nodes[nodeIndex].next[key] = next
				matcher.nodes = append(matcher.nodes, foldMatcherNode{minRule: -1})
			}
			nodeIndex = next
		}
		if matcher.nodes[nodeIndex].minRule < 0 || ruleIndex < matcher.nodes[nodeIndex].minRule {
			matcher.nodes[nodeIndex].minRule = ruleIndex
		}
	}

	queue := make([]int, 0, len(matcher.nodes))
	for _, child := range matcher.nodes[0].next {
		queue = append(queue, child)
	}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		for key, child := range matcher.nodes[current].next {
			failure := matcher.nodes[current].fail
			for {
				if next, ok := matcher.nodes[failure].next[key]; ok {
					failure = next
					break
				}
				if failure == 0 {
					break
				}
				failure = matcher.nodes[failure].fail
			}
			matcher.nodes[child].fail = failure
			if inherited := matcher.nodes[failure].minRule; inherited >= 0 && (matcher.nodes[child].minRule < 0 || inherited < matcher.nodes[child].minRule) {
				matcher.nodes[child].minRule = inherited
			}
			queue = append(queue, child)
		}
	}
	return matcher
}

func (matcher *foldMatcher) match(text string) (int, bool) {
	if matcher == nil || len(matcher.nodes) == 0 {
		return -1, false
	}
	state := 0
	bestRule := -1
	for position := 0; position < len(text); {
		got, size := utf8.DecodeRuneInString(text[position:])
		key := foldClassRune(got)
		for {
			next, ok := matcher.nodes[state].next[key]
			if ok {
				state = next
				break
			}
			if state == 0 {
				break
			}
			state = matcher.nodes[state].fail
		}
		if ruleIndex := matcher.nodes[state].minRule; ruleIndex == 0 {
			return 0, true
		} else if ruleIndex >= 0 && (bestRule < 0 || ruleIndex < bestRule) {
			bestRule = ruleIndex
		}
		position += size
	}
	return bestRule, bestRule >= 0
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

func rewriteFolded(text string, runes []rune, char string, obfuscate bool) (string, bool) {
	if len(runes) == 0 {
		return text, false
	}
	var out strings.Builder
	position := 0
	matched := false
	for start := 0; start < len(text); {
		end, ok := foldMatchEnd(text, start, runes)
		if !ok {
			_, size := utf8.DecodeRuneInString(text[start:])
			start += size
			continue
		}
		if !matched {
			capacity := len(text)
			if obfuscate {
				capacity += len(text) / len(runes) * len(char)
			}
			out.Grow(capacity)
			matched = true
		}
		out.WriteString(text[position:start])
		if obfuscate {
			_, firstSize := utf8.DecodeRuneInString(text[start:end])
			out.WriteString(text[start : start+firstSize])
			out.WriteString(char)
			out.WriteString(text[start+firstSize : end])
		}
		position = end
		start = end
	}
	if !matched {
		return text, false
	}
	out.WriteString(text[position:])
	return out.String(), true
}

func stripFoldRule(text string, runes []rune) (string, bool) {
	return rewriteFolded(text, runes, "", false)
}

func obfuscateFoldRule(text string, runes []rune, char string) (string, bool) {
	return rewriteFolded(text, runes, char, true)
}

func stripRule(text string, rule compiledRule, ignoreCase bool) (string, bool) {
	if !ignoreCase {
		if !strings.Contains(text, rule.Term) {
			return text, false
		}
		return strings.ReplaceAll(text, rule.Term, ""), true
	}
	return stripFoldRule(text, rule.Runes)
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
	return obfuscateFoldRule(text, rule.Runes, char)
}
