package main

import "github.com/tidwall/gjson"

func collectGemini(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	collectParts := func(parts gjson.Result, role string) {
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

	collectParts(root.Get("systemInstruction.parts"), "system")
	collectParts(root.Get("system_instruction.parts"), "system")

	contents := root.Get("contents")
	if !contents.IsArray() {
		return
	}
	previousRole := ""
	contents.ForEach(func(_, content gjson.Result) bool {
		role := content.Get("role")
		switch {
		case !role.Exists():
			effectiveRole := nextGeminiRole(previousRole)
			previousRole = effectiveRole
			if effectiveRole == "user" {
				collectParts(content.Get("parts"), "user")
			} else {
				collectParts(content.Get("parts"), "assistant")
			}
		case role.Type == gjson.String && role.Str == "user":
			previousRole = "user"
			collectParts(content.Get("parts"), "user")
		case role.Type == gjson.String && role.Str == "model":
			previousRole = "model"
			collectParts(content.Get("parts"), "assistant")
		default:
			previousRole = nextGeminiRole(previousRole)
		}
		return true
	})
}

func nextGeminiRole(previousRole string) string {
	if previousRole == "" || previousRole == "model" {
		return "user"
	}
	return "model"
}
