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

type byteMatcherNode struct {
	next    map[byte]int
	fail    int
	minRule int
}

type byteMatcher struct {
	nodes     []byteMatcherNode
	root      [256]int
	firstRule int
}

func (matcher *byteMatcher) transition(state int, key byte) (int, bool) {
	if state == 0 {
		next := matcher.root[key]
		return next, next != 0
	}
	next, ok := matcher.nodes[state].next[key]
	return next, ok
}

func (matcher *byteMatcher) setTransition(state int, key byte, next int) {
	if state == 0 {
		matcher.root[key] = next
		return
	}
	if matcher.nodes[state].next == nil {
		matcher.nodes[state].next = make(map[byte]int)
	}
	matcher.nodes[state].next[key] = next
}

func newByteMatcher(rules []compiledRule, ruleOffset int) *byteMatcher {
	matcher := &byteMatcher{
		nodes:     []byteMatcherNode{{minRule: -1}},
		firstRule: ruleOffset,
	}
	for localIndex, rule := range rules {
		if rule.Term == "" {
			continue
		}
		state := 0
		for i := 0; i < len(rule.Term); i++ {
			key := rule.Term[i]
			next, ok := matcher.transition(state, key)
			if !ok {
				next = len(matcher.nodes)
				matcher.setTransition(state, key, next)
				matcher.nodes = append(matcher.nodes, byteMatcherNode{minRule: -1})
			}
			state = next
		}
		ruleIndex := ruleOffset + localIndex
		if matcher.nodes[state].minRule < 0 || ruleIndex < matcher.nodes[state].minRule {
			matcher.nodes[state].minRule = ruleIndex
		}
	}

	queue := make([]int, 0, len(matcher.nodes))
	for _, child := range matcher.root {
		if child != 0 {
			queue = append(queue, child)
		}
	}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		for key, child := range matcher.nodes[current].next {
			failure := matcher.nodes[current].fail
			for {
				if next, ok := matcher.transition(failure, key); ok {
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

func (matcher *byteMatcher) match(text string) (int, bool) {
	if matcher == nil || len(matcher.nodes) == 0 {
		return -1, false
	}
	state := 0
	bestRule := -1
	for i := 0; i < len(text); i++ {
		key := text[i]
		for {
			next, ok := matcher.transition(state, key)
			if ok {
				state = next
				break
			}
			if state == 0 {
				break
			}
			state = matcher.nodes[state].fail
		}
		if ruleIndex := matcher.nodes[state].minRule; ruleIndex == matcher.firstRule {
			return ruleIndex, true
		} else if ruleIndex >= 0 && (bestRule < 0 || ruleIndex < bestRule) {
			bestRule = ruleIndex
		}
	}
	return bestRule, bestRule >= 0
}

type foldMatcherNode struct {
	next    map[rune]int
	fail    int
	minRule int
}

type foldMatcher struct {
	nodes     []foldMatcherNode
	asciiRoot [utf8.RuneSelf]int
	firstRule int
}

func (matcher *foldMatcher) transition(state int, key rune) (int, bool) {
	if state == 0 && key >= 0 && key < utf8.RuneSelf {
		next := matcher.asciiRoot[key]
		return next, next != 0
	}
	next, ok := matcher.nodes[state].next[key]
	return next, ok
}

func (matcher *foldMatcher) setTransition(state int, key rune, next int) {
	if state == 0 && key >= 0 && key < utf8.RuneSelf {
		matcher.asciiRoot[key] = next
		return
	}
	if matcher.nodes[state].next == nil {
		matcher.nodes[state].next = make(map[rune]int)
	}
	matcher.nodes[state].next[key] = next
}

func newFoldMatcher(rules []compiledRule, ruleOffsets ...int) *foldMatcher {
	ruleOffset := 0
	if len(ruleOffsets) != 0 {
		ruleOffset = ruleOffsets[0]
	}
	matcher := &foldMatcher{
		nodes:     []foldMatcherNode{{minRule: -1}},
		firstRule: ruleOffset,
	}
	for localIndex, rule := range rules {
		if len(rule.Runes) == 0 {
			continue
		}
		nodeIndex := 0
		for _, r := range rule.Runes {
			key := foldClassRune(r)
			next, ok := matcher.transition(nodeIndex, key)
			if !ok {
				next = len(matcher.nodes)
				matcher.setTransition(nodeIndex, key, next)
				matcher.nodes = append(matcher.nodes, foldMatcherNode{minRule: -1})
			}
			nodeIndex = next
		}
		ruleIndex := ruleOffset + localIndex
		if matcher.nodes[nodeIndex].minRule < 0 || ruleIndex < matcher.nodes[nodeIndex].minRule {
			matcher.nodes[nodeIndex].minRule = ruleIndex
		}
	}

	queue := make([]int, 0, len(matcher.nodes))
	for _, child := range matcher.asciiRoot {
		if child != 0 {
			queue = append(queue, child)
		}
	}
	for _, child := range matcher.nodes[0].next {
		queue = append(queue, child)
	}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		for key, child := range matcher.nodes[current].next {
			failure := matcher.nodes[current].fail
			for {
				if next, ok := matcher.transition(failure, key); ok {
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
			next, ok := matcher.transition(state, key)
			if ok {
				state = next
				break
			}
			if state == 0 {
				break
			}
			state = matcher.nodes[state].fail
		}
		if ruleIndex := matcher.nodes[state].minRule; ruleIndex == matcher.firstRule {
			return ruleIndex, true
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

func buildFoldFailure(pattern []rune) []int {
	failure := make([]int, len(pattern))
	for i, prefix := 1, 0; i < len(pattern); i++ {
		for prefix > 0 && pattern[i] != pattern[prefix] {
			prefix = failure[prefix-1]
		}
		if pattern[i] == pattern[prefix] {
			prefix++
		}
		failure[i] = prefix
	}
	return failure
}

func rewriteFoldedKMP(text string, rule compiledRule, char string, obfuscate bool) (string, bool) {
	pattern := rule.Runes
	if len(pattern) == 0 || len(rule.FoldFailure) != len(pattern) {
		return text, false
	}
	var out strings.Builder
	written := 0
	state := 0
	matched := false
	for scan := 0; scan < len(text); {
		got, size := utf8.DecodeRuneInString(text[scan:])
		key := foldClassRune(got)
		for state > 0 && key != pattern[state] {
			state = rule.FoldFailure[state-1]
		}
		if key == pattern[state] {
			state++
		}
		scan += size
		if state != len(pattern) {
			continue
		}

		start := scan
		for range pattern {
			_, size = utf8.DecodeLastRuneInString(text[:start])
			start -= size
		}
		if !matched && start == 0 && scan < len(text) {
			if _, adjacent := foldMatchEnd(text, scan, pattern); adjacent {
				return rewriteFolded(text, pattern, char, obfuscate)
			}
		}
		if !matched {
			capacity := len(text)
			if obfuscate {
				capacity += len(text) / len(pattern) * len(char)
			}
			out.Grow(capacity)
			matched = true
		}
		out.WriteString(text[written:start])
		if obfuscate {
			_, firstSize := utf8.DecodeRuneInString(text[start:scan])
			out.WriteString(text[start : start+firstSize])
			out.WriteString(char)
			out.WriteString(text[start+firstSize : scan])
		}
		written = scan
		state = 0
	}
	if !matched {
		return text, false
	}
	out.WriteString(text[written:])
	return out.String(), true
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

func rewriteExact(text string, rule compiledRule, obfuscate bool) (string, bool) {
	replacement := ""
	if obfuscate {
		replacement = rule.ExactReplacement
	}
	rewritten := strings.ReplaceAll(text, rule.Term, replacement)
	if obfuscate {
		return rewritten, len(rewritten) > len(text)
	}
	return rewritten, len(rewritten) < len(text)
}

func stripRule(text string, rule compiledRule, ignoreCase bool) (string, bool) {
	if !ignoreCase {
		return rewriteExact(text, rule, false)
	}
	if len(rule.FoldFailure) != 0 && len(text) >= foldKMPMinTextBytes {
		return rewriteFoldedKMP(text, rule, "", false)
	}
	return stripFoldRule(text, rule.Runes)
}

func obfuscateRule(text string, rule compiledRule, ignoreCase bool, char string) (string, bool) {
	if !ignoreCase {
		return rewriteExact(text, rule, true)
	}
	if len(rule.FoldFailure) != 0 && len(text) >= foldKMPMinTextBytes {
		return rewriteFoldedKMP(text, rule, char, true)
	}
	return obfuscateFoldRule(text, rule.Runes, char)
}
