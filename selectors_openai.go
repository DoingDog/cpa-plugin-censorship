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
		roleName := role.Str
		if roleName == "function" {
			roleName = "tool"
		}
		switch roleName {
		case "system", "developer", "user", "assistant", "tool":
		default:
			return true
		}
		if !roles.has(roleName) {
			return true
		}
		content := message.Get("content")
		appendStringSpan(spans, content, roleName, roles)
		if roleName == "assistant" {
			appendStringSpan(spans, message.Get("refusal"), roleName, roles)
		}
		if content.IsArray() {
			content.ForEach(func(_, part gjson.Result) bool {
				partType := part.Get("type")
				if partType.Type != gjson.String {
					return true
				}
				switch partType.Str {
				case "text":
					appendStringSpan(spans, part.Get("text"), roleName, roles)
				case "refusal":
					if roleName == "assistant" {
						appendStringSpan(spans, part.Get("refusal"), roleName, roles)
					}
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

func appendOpenAIResponsesInputTextOutput(output gjson.Result, roles scopeSet, spans *[]textSpan) {
	appendStringSpan(spans, output, "tool", roles)
	if !output.IsArray() {
		return
	}
	output.ForEach(func(_, part gjson.Result) bool {
		partType := part.Get("type")
		if partType.Type == gjson.String && partType.Str == "input_text" {
			appendStringSpan(spans, part.Get("text"), "tool", roles)
		}
		return true
	})
}

func collectOpenAIResponsesToolOutput(item gjson.Result, roles scopeSet, spans *[]textSpan) bool {
	itemType := item.Get("type")
	if itemType.Type != gjson.String {
		return false
	}

	switch itemType.Str {
	case "function_call_output", "custom_tool_call_output":
		if roles.has("tool") {
			appendOpenAIResponsesInputTextOutput(item.Get("output"), roles, spans)
		}
		return true
	case "local_shell_call_output", "apply_patch_call_output":
		if roles.has("tool") {
			appendStringSpan(spans, item.Get("output"), "tool", roles)
		}
		return true
	case "shell_call_output":
		if roles.has("tool") {
			output := item.Get("output")
			if output.IsArray() {
				output.ForEach(func(_, result gjson.Result) bool {
					appendStringSpan(spans, result.Get("stdout"), "tool", roles)
					appendStringSpan(spans, result.Get("stderr"), "tool", roles)
					return true
				})
			}
		}
		return true
	case "mcp_call":
		if roles.has("tool") {
			appendStringSpan(spans, item.Get("output"), "tool", roles)
			appendStringSpan(spans, item.Get("error"), "tool", roles)
		}
		return true
	case "program_result":
		if roles.has("tool") {
			appendStringSpan(spans, item.Get("result"), "tool", roles)
		}
		return true
	default:
		return false
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
		if !item.IsObject() {
			return true
		}
		if collectOpenAIResponsesToolOutput(item, roles, spans) || !openAIResponsesMessageType(item) {
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
