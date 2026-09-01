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
		case "system", "developer", "user", "assistant":
			content := message.Get("content")
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
		case "tool":
			appendStringSpan(spans, message.Get("content"), role.Str, roles)
		}
		return true
	})
}

func collectOpenAIResponses(root gjson.Result, roles scopeSet, spans *[]textSpan) {}
