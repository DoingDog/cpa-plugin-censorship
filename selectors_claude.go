package main

import "github.com/tidwall/gjson"

func collectClaude(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	if roles.has("system") {
		system := root.Get("system")
		appendStringSpan(spans, system, "system", roles)
		if system.IsArray() {
			system.ForEach(func(_, block gjson.Result) bool {
				if block.IsObject() {
					blockType := block.Get("type")
					if blockType.Type == gjson.String && blockType.Str == "text" {
						appendStringSpan(spans, block.Get("text"), "system", roles)
					}
				}
				return true
			})
		}
	}

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
		case "system", "assistant":
			if !roles.has(role.Str) {
				return true
			}
		case "user":
			if !roles.has("user") && !roles.has("tool") {
				return true
			}
		default:
			return true
		}

		content := message.Get("content")
		appendStringSpan(spans, content, role.Str, roles)
		if !content.IsArray() {
			return true
		}
		content.ForEach(func(_, block gjson.Result) bool {
			if !block.IsObject() {
				return true
			}
			blockType := block.Get("type")
			if blockType.Type != gjson.String {
				return true
			}
			switch blockType.Str {
			case "text":
				if roles.has(role.Str) {
					appendStringSpan(spans, block.Get("text"), role.Str, roles)
				}
			case "search_result", "document":
				if role.Str == "user" {
					appendClaudeResultText(spans, block, "user", roles)
				}
			case "tool_result":
				if role.Str != "user" || !roles.has("tool") {
					return true
				}
				toolContent := block.Get("content")
				appendStringSpan(spans, toolContent, "tool", roles)
				if !toolContent.IsArray() {
					return true
				}
				toolContent.ForEach(func(_, inner gjson.Result) bool {
					appendClaudeResultText(spans, inner, "tool", roles)
					if inner.IsObject() {
						innerType := inner.Get("type")
						if innerType.Type == gjson.String && innerType.Str == "text" {
							appendStringSpan(spans, inner.Get("text"), "tool", roles)
						}
					}
					return true
				})
			}
			return true
		})
		return true
	})
}

func appendClaudeResultText(spans *[]textSpan, block gjson.Result, role string, roles scopeSet) {
	if !roles.has(role) || !block.IsObject() {
		return
	}

	blockType := block.Get("type")
	if blockType.Type != gjson.String {
		return
	}

	switch blockType.Str {
	case "search_result":
		content := block.Get("content")
		if !content.IsArray() {
			return
		}
		content.ForEach(func(_, part gjson.Result) bool {
			if part.IsObject() && part.Get("type").Str == "text" {
				appendStringSpan(spans, part.Get("text"), role, roles)
			}
			return true
		})
	case "document":
		source := block.Get("source")
		if !source.IsObject() || source.Get("type").Str != "text" {
			return
		}
		appendStringSpan(spans, source.Get("data"), role, roles)
	}
}
