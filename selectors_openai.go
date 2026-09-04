package main

import "github.com/tidwall/gjson"

func collectOpenAI(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	messages := root.Get("messages")
	if !messages.IsArray() {
		return
	}
	messages.ForEach(func(_, message gjson.Result) bool {
		if !message.IsObject() {
			return true
		}
		role := message.Get("role")
		if role.Type != gjson.String {
			return true
		}
		switch role.Str {
		case "system", "developer", "user", "assistant", "tool":
		default:
			return true
		}
		if !roles.has(role.Str) {
			return true
		}
		content := message.Get("content")
		if role.Str == "tool" {
			appendStringSpan(spans, content, role.Str, roles)
			return true
		}
		appendStringSpan(spans, content, role.Str, roles)
		if content.IsArray() {
			content.ForEach(func(_, part gjson.Result) bool {
				partType := part.Get("type")
				if partType.Type == gjson.String && partType.Str == "text" {
					appendStringSpan(spans, part.Get("text"), role.Str, roles)
				}
				return true
			})
		}
		return true
	})
}

func openAIResponsesMessageType(item gjson.Result) bool {
	itemType := item.Get("type")
	return !itemType.Exists() || itemType.Type == gjson.String && (itemType.Str == "" || itemType.Str == "message")
}

func openAIResponsesRole(item gjson.Result) (string, bool) {
	role := item.Get("role")
	if role.Type != gjson.String {
		return "", false
	}
	switch role.Str {
	case "system", "developer", "user", "assistant":
		return role.Str, true
	default:
		return "", false
	}
}

func collectOpenAIResponses(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	if roles.has("system") {
		appendStringSpan(spans, root.Get("instructions"), "system", roles)
	}
	input := root.Get("input")
	if input.Type == gjson.String && roles.has("user") {
		appendStringSpan(spans, input, "user", roles)
	}
	if !input.IsArray() {
		return
	}
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() || !openAIResponsesMessageType(item) {
			return true
		}
		role, ok := openAIResponsesRole(item)
		if !ok || !roles.has(role) && !roles.has("assistant") {
			return true
		}
		content := item.Get("content")
		appendStringSpan(spans, content, role, roles)
		if content.IsArray() {
			content.ForEach(func(_, part gjson.Result) bool {
				partType := part.Get("type")
				if partType.Type == gjson.String && partType.Str == "refusal" {
					if roles.has("assistant") {
						appendStringSpan(spans, part.Get("refusal"), "assistant", roles)
					}
					return true
				}
				if !partType.Exists() || partType.Type == gjson.String && (partType.Str == "" || partType.Str == "input_text" || partType.Str == "output_text") {
					partRole := role
					if partType.Str == "output_text" {
						partRole = "assistant"
					}
					if roles.has(partRole) {
						appendStringSpan(spans, part.Get("text"), partRole, roles)
					}
				}
				return true
			})
		}
		return true
	})
}
