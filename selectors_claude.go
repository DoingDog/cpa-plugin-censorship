package main

import "github.com/tidwall/gjson"

func collectClaude(root gjson.Result, roles scopeSet, spans *[]textSpan) {
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
		case "system", "user", "assistant":
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
				appendStringSpan(spans, block.Get("text"), role.Str, roles)
			case "tool_result":
				if role.Str != "user" {
					return true
				}
				toolContent := block.Get("content")
				if !toolContent.IsArray() {
					return true
				}
				toolContent.ForEach(func(_, inner gjson.Result) bool {
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
