package main

import "github.com/tidwall/gjson"

func collectInteractions(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	systemInstruction := root.Get("system_instruction")
	switch {
	case systemInstruction.Type == gjson.String:
		appendStringSpan(spans, systemInstruction, "system", roles)
	case systemInstruction.IsObject():
		appendStringSpan(spans, systemInstruction.Get("text"), "system", roles)
		parts := systemInstruction.Get("parts")
		if parts.IsArray() {
			parts.ForEach(func(_, part gjson.Result) bool {
				if interactionTextPartAllowed(part) && geminiTextPartAllowed(part) {
					appendStringSpan(spans, part.Get("text"), "system", roles)
				}
				return true
			})
		}
	}

	input := root.Get("input")
	switch {
	case input.Type == gjson.String:
		appendStringSpan(spans, input, "user", roles)
	case input.IsObject():
		collectInteractionItem(input, "user", roles, spans)
	case input.IsArray():
		input.ForEach(func(_, item gjson.Result) bool {
			if item.Type == gjson.String {
				appendStringSpan(spans, item, "user", roles)
			} else if item.IsObject() {
				collectInteractionItem(item, "user", roles, spans)
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

	content := item.Get("content")
	switch {
	case content.Type == gjson.String:
		appendStringSpan(spans, content, role, roles)
	case content.IsObject():
		if interactionTextPartAllowed(content) && geminiTextPartAllowed(content) {
			appendStringSpan(spans, content.Get("text"), role, roles)
		}
	case content.IsArray():
		content.ForEach(func(_, part gjson.Result) bool {
			if interactionTextPartAllowed(part) && geminiTextPartAllowed(part) {
				appendStringSpan(spans, part.Get("text"), role, roles)
			}
			return true
		})
	}

	parts := item.Get("parts")
	if parts.IsArray() {
		parts.ForEach(func(_, part gjson.Result) bool {
			if interactionTextPartAllowed(part) && geminiTextPartAllowed(part) {
				appendStringSpan(spans, part.Get("text"), role, roles)
			}
			return true
		})
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

func interactionTextPartAllowed(part gjson.Result) bool {
	if !part.IsObject() {
		return false
	}
	partType := part.Get("type")
	return !partType.Exists() || partType.Type == gjson.String && (partType.Str == "" || partType.Str == "text")
}
