package main

import "strings"

type filterMode string

const (
	filterModeExclude filterMode = "exclude"
	filterModeInclude filterMode = "include"
)

type filterLogic string

const (
	filterLogicOr  filterLogic = "or"
	filterLogicAnd filterLogic = "and"
)

type compiledFilterPattern struct {
	Text        string
	Runes       []rune
	CallerScope string
}

type requestFilter struct {
	Mode    filterMode
	Logic   filterLogic
	APIKeys []compiledFilterPattern
	Models  []compiledFilterPattern
}

func (f requestFilter) enabled() bool {
	return len(f.APIKeys) != 0 || len(f.Models) != 0
}

func compileRequestFilter(filter *requestFilter) {
	for i := range filter.APIKeys {
		pattern := &filter.APIKeys[i]
		pattern.Runes = nil
		pattern.CallerScope = ""
		if strings.ContainsAny(pattern.Text, "*?") {
			pattern.Runes = []rune(pattern.Text)
		}
	}
	for i := range filter.Models {
		pattern := &filter.Models[i]
		pattern.Runes = nil
		pattern.CallerScope = ""
		if strings.ContainsAny(pattern.Text, "*?") {
			pattern.Runes = []rune(pattern.Text)
		}
	}
}
