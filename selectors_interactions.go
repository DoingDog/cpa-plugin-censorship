package main

import "github.com/tidwall/gjson"

func appendInteractionTextPart(spans *[]textSpan, part gjson.Result, role string, roles scopeSet, protectAnnotations bool) {
	text, allowed, _ := scanTextPart(part, true)
	if !allowed {
		return
	}
	before := len(*spans)
	appendStringSpan(spans, text, role, roles)
	annotations := part.Get("annotations")
	if protectAnnotations && len(*spans) != before && annotations.IsArray() && annotations.Get("#").Int() > 0 {
		span := &(*spans)[len(*spans)-1]
		span.RequiresUnmodified = true
		span.UnmodifiedMessage = "censorship cannot rewrite annotated text"
	}
}

func collectInteractionResult(item gjson.Result, roles scopeSet, spans *[]textSpan) bool {
	itemType := item.Get("type")
	if itemType.Type != gjson.String {
		return false
	}
	switch itemType.Str {
	case "function_result", "mcp_server_tool_result":
		appendInteractionResult(spans, item.Get("result"), roles)
		return true
	case "code_execution_result":
		before := len(*spans)
		appendStringSpan(spans, item.Get("result"), "tool", roles)
		if len(*spans) != before && item.Get("signature").Type != gjson.Null {
			(*spans)[len(*spans)-1].RequiresUnmodified = true
		}
		return true
	default:
		return false
	}
}

func appendInteractionResult(spans *[]textSpan, result gjson.Result, roles scopeSet) {
	if !roles.has("tool") {
		return
	}
	if result.Type == gjson.String {
		appendStringSpan(spans, result, "tool", roles)
		return
	}
	if !result.IsArray() {
		return
	}
	result.ForEach(func(_, part gjson.Result) bool {
		partType := part.Get("type")
		if part.IsObject() && partType.Type == gjson.String && partType.Str == "text" {
			appendInteractionTextPart(spans, part, "tool", roles, false)
		}
		return true
	})
}

func collectInteractionDefinitions(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	if !roles.has("developer") {
		return
	}
	tools := root.Get("tools")
	if tools.IsArray() {
		tools.ForEach(func(_, tool gjson.Result) bool {
			toolType := tool.Get("type")
			if tool.IsObject() && toolType.Type == gjson.String && toolType.Str == "function" {
				appendStringSpan(spans, tool.Get("description"), "developer", roles)
				appendJSONSchemaDescriptions(spans, tool.Get("parameters"), "developer", roles)
			}
			return true
		})
	}
	format := root.Get("response_format")
	append := func(candidate gjson.Result) {
		candidateType := candidate.Get("type")
		if candidate.IsObject() && candidateType.Type == gjson.String && candidateType.Str == "text" {
			appendJSONSchemaDescriptions(spans, candidate.Get("schema"), "developer", roles)
		}
	}
	if format.IsArray() {
		format.ForEach(func(_, candidate gjson.Result) bool { append(candidate); return true })
	} else {
		append(format)
	}
}

func collectInteractions(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	collectInteractionDefinitions(root, roles, spans)

	if roles.has("system") {
		systemInstruction := root.Get("system_instruction")
		if !systemInstruction.Exists() {
			systemInstruction = root.Get("systemInstruction")
		}
		switch {
		case systemInstruction.Type == gjson.String:
			appendStringSpan(spans, systemInstruction, "system", roles)
		case systemInstruction.IsObject():
			appendInteractionTextPart(spans, systemInstruction, "system", roles, false)
			parts := systemInstruction.Get("parts")
			if parts.IsArray() {
				parts.ForEach(func(_, part gjson.Result) bool {
					appendInteractionTextPart(spans, part, "system", roles, false)
					return true
				})
			}
		}
	}

	if !roles.has("user") && !roles.has("assistant") && !roles.has("tool") {
		return
	}
	input := root.Get("input")
	switch {
	case input.Type == gjson.String:
		appendStringSpan(spans, input, "user", roles)
	case input.IsObject():
		if input.Get("type").String() == "text" {
			appendInteractionTextPart(spans, input, "user", roles, false)
		} else {
			collectInteractionItem(input, "user", roles, spans)
		}
	case input.IsArray():
		input.ForEach(func(_, item gjson.Result) bool {
			if item.Type == gjson.String {
				appendStringSpan(spans, item, "user", roles)
			} else if item.IsObject() {
				if item.Get("type").String() == "text" {
					appendInteractionTextPart(spans, item, "user", roles, false)
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
	if collectInteractionResult(item, roles, spans) {
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

	protectAnnotations := itemType.Type == gjson.String && itemType.Str == "model_output"
	if roles.has(role) {
		content := item.Get("content")
		switch {
		case content.Type == gjson.String:
			appendStringSpan(spans, content, role, roles)
		case content.IsObject():
			appendInteractionTextPart(spans, content, role, roles, protectAnnotations)
		case content.IsArray():
			content.ForEach(func(_, part gjson.Result) bool {
				appendInteractionTextPart(spans, part, role, roles, protectAnnotations)
				return true
			})
		}

		parts := item.Get("parts")
		if parts.IsArray() {
			parts.ForEach(func(_, part gjson.Result) bool {
				appendInteractionTextPart(spans, part, role, roles, false)
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
