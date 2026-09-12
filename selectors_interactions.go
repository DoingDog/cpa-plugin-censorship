package main

import "github.com/tidwall/gjson"

func collectInteractions(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	if roles.has("system") {
		systemInstruction := root.Get("system_instruction")
		if !systemInstruction.Exists() {
			systemInstruction = root.Get("systemInstruction")
		}
		switch {
		case systemInstruction.Type == gjson.String:
			appendStringSpan(spans, systemInstruction, "system", roles)
		case systemInstruction.IsObject():
			text, allowed, _ := scanTextPart(systemInstruction, true)
			if allowed {
				appendStringSpan(spans, text, "system", roles)
			}
			parts := systemInstruction.Get("parts")
			if parts.IsArray() {
				parts.ForEach(func(_, part gjson.Result) bool {
					text, allowed, _ := scanTextPart(part, true)
					if allowed {
						appendStringSpan(spans, text, "system", roles)
					}
					return true
				})
			}
		}
	}

	if !roles.has("user") && !roles.has("assistant") {
		return
	}
	input := root.Get("input")
	switch {
	case input.Type == gjson.String:
		appendStringSpan(spans, input, "user", roles)
	case input.IsObject():
		if input.Get("type").String() == "text" {
			text, allowed, _ := scanTextPart(input, true)
			if allowed {
				appendStringSpan(spans, text, "user", roles)
			}
		} else {
			collectInteractionItem(input, "user", roles, spans)
		}
	case input.IsArray():
		input.ForEach(func(_, item gjson.Result) bool {
			if item.Type == gjson.String {
				appendStringSpan(spans, item, "user", roles)
			} else if item.IsObject() {
				if item.Get("type").String() == "text" {
					text, allowed, _ := scanTextPart(item, true)
					if allowed {
						appendStringSpan(spans, text, "user", roles)
					}
				} else {
					collectInteractionItem(item, "user", roles, spans)
				}
			}
			return true
		})
	}
}

func collectInteractionItem(item gjson.Result, inheritedRole string, roles scopeSet, spans *[]textSpan) {
	if !item.IsObject() {
		return
	}

	role := inheritedRole
	itemRole := item.Get("role")
	if itemRole.Exists() {
		if itemRole.Type != gjson.String {
			return
		}
		switch itemRole.Str {
		case "user":
			role = "user"
		case "model", "assistant":
			role = "assistant"
		default:
			return
		}
	}

	itemType := item.Get("type")
	if itemType.Exists() {
		if itemType.Type != gjson.String {
			return
		}
		switch itemType.Str {
		case "", "user_input":
		case "model_output":
			role = "assistant"
		default:
			return
		}
	}

	if roles.has(role) {
		content := item.Get("content")
		switch {
		case content.Type == gjson.String:
			appendStringSpan(spans, content, role, roles)
		case content.IsObject():
			text, allowed, _ := scanTextPart(content, true)
			if allowed {
				appendStringSpan(spans, text, role, roles)
			}
		case content.IsArray():
			content.ForEach(func(_, part gjson.Result) bool {
				text, allowed, _ := scanTextPart(part, true)
				if allowed {
					appendStringSpan(spans, text, role, roles)
				}
				return true
			})
		}

		parts := item.Get("parts")
		if parts.IsArray() {
			parts.ForEach(func(_, part gjson.Result) bool {
				text, allowed, _ := scanTextPart(part, true)
				if allowed {
					appendStringSpan(spans, text, role, roles)
				}
				return true
			})
		}
	}

	steps := item.Get("steps")
	if steps.IsArray() {
		steps.ForEach(func(_, step gjson.Result) bool {
			if step.IsObject() {
				collectInteractionItem(step, role, roles, spans)
			}
			return true
		})
	}
}
