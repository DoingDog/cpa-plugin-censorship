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
			appendStringSpan(spans, message.Get("content"), role.Str, roles)
		}
		return true
	})
}

func collectOpenAIResponses(root gjson.Result, roles scopeSet, spans *[]textSpan) {}
