package main

import "strings"

func containsRule(text string, rule compiledRule, ignoreCase bool) bool {
	if ignoreCase {
		return false
	}
	return strings.Contains(text, rule.Term)
}
