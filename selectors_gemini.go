package main

import "github.com/tidwall/gjson"

func collectGemini(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	collectParts := func(parts gjson.Result, role string) {
		if !parts.IsArray() {
			return
		}
		parts.ForEach(func(_, part gjson.Result) bool {
			if geminiTextPartAllowed(part) {
				appendStringSpan(spans, part.Get("text"), role, roles)
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
	contents.ForEach(func(_, content gjson.Result) bool {
		role := content.Get("role")
		switch {
		case !role.Exists():
			collectParts(content.Get("parts"), "user")
		case role.Type == gjson.String && role.Str == "user":
			collectParts(content.Get("parts"), "user")
		case role.Type == gjson.String && role.Str == "model":
			collectParts(content.Get("parts"), "assistant")
		}
		return true
	})
}
