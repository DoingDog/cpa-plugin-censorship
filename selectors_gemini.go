package main

import "github.com/tidwall/gjson"

func collectGemini(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	collectParts := func(content gjson.Result, role string) {
		if !roles.has(role) {
			return
		}
		parts := content.Get("parts")
		if !parts.IsArray() {
			return
		}
		parts.ForEach(func(_, part gjson.Result) bool {
			text, allowed := scanTextPart(part, false)
			if allowed {
				appendStringSpan(spans, text, role, roles)
			}
			return true
		})
	}

	if roles.has("system") {
		collectParts(root.Get("systemInstruction"), "system")
		collectParts(root.Get("system_instruction"), "system")
	}
	if !roles.has("user") && !roles.has("assistant") {
		return
	}

	contents := root.Get("contents")
	if !contents.IsArray() {
		return
	}
	previousRole := ""
	contents.ForEach(func(_, content gjson.Result) bool {
		role := content.Get("role")
		var effectiveRole string
		switch {
		case !role.Exists() || role.Type == gjson.Null || role.String() == "":
			previousRole = nextGeminiRole(previousRole)
			if previousRole == "user" {
				effectiveRole = "user"
			} else {
				effectiveRole = "assistant"
			}
		case role.Type == gjson.String && role.Str == "user":
			previousRole = "user"
			effectiveRole = "user"
		case role.Type == gjson.String && role.Str == "model":
			previousRole = "model"
			effectiveRole = "assistant"
		default:
			previousRole = nextGeminiRole(previousRole)
			return true
		}
		collectParts(content, effectiveRole)
		return true
	})
}

func nextGeminiRole(previousRole string) string {
	if previousRole == "" || previousRole == "model" {
		return "user"
	}
	return "model"
}
