package main

import "github.com/tidwall/gjson"

func appendJSONSchemaDescriptions(spans *[]textSpan, schema gjson.Result, role string, roles scopeSet) {
	if !schema.IsObject() || !roles.has(role) {
		return
	}
	appendStringSpan(spans, schema.Get("description"), role, roles)
	for _, key := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"} {
		children := schema.Get(key)
		if children.IsObject() {
			children.ForEach(func(_, child gjson.Result) bool {
				appendJSONSchemaDescriptions(spans, child, role, roles)
				return true
			})
		}
	}
	for _, key := range []string{"items", "additionalProperties", "unevaluatedProperties", "propertyNames", "contains", "not", "if", "then", "else"} {
		appendJSONSchemaDescriptions(spans, schema.Get(key), role, roles)
	}
	for _, key := range []string{"prefixItems", "allOf", "anyOf", "oneOf"} {
		children := schema.Get(key)
		if children.IsArray() {
			children.ForEach(func(_, child gjson.Result) bool {
				appendJSONSchemaDescriptions(spans, child, role, roles)
				return true
			})
		}
	}
}

func appendOpenAIChatTool(spans *[]textSpan, tool gjson.Result, roles scopeSet) {
	if !tool.IsObject() || !roles.has("developer") {
		return
	}
	switch tool.Get("type").Str {
	case "function":
		function := tool.Get("function")
		appendStringSpan(spans, function.Get("description"), "developer", roles)
		appendJSONSchemaDescriptions(spans, function.Get("parameters"), "developer", roles)
		appendJSONSchemaDescriptions(spans, function.Get("output_schema"), "developer", roles)
	case "custom":
		appendStringSpan(spans, tool.Get("custom.description"), "developer", roles)
	}
}

func collectOpenAI(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	messages := root.Get("messages")
	if messages.IsArray() {
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
	if roles.has("assistant") && root.Get("prediction.type").Str == "content" {
		prediction := root.Get("prediction.content")
		appendStringSpan(spans, prediction, "assistant", roles)
		if prediction.IsArray() {
			prediction.ForEach(func(_, part gjson.Result) bool {
				if part.Get("type").Str == "text" {
					appendStringSpan(spans, part.Get("text"), "assistant", roles)
				}
				return true
			})
		}
	}
	if roles.has("developer") {
		functions := root.Get("functions")
		if functions.IsArray() {
			functions.ForEach(func(_, function gjson.Result) bool {
				appendStringSpan(spans, function.Get("description"), "developer", roles)
				appendJSONSchemaDescriptions(spans, function.Get("parameters"), "developer", roles)
				appendJSONSchemaDescriptions(spans, function.Get("output_schema"), "developer", roles)
				return true
			})
		}
		tools := root.Get("tools")
		if tools.IsArray() {
			tools.ForEach(func(_, tool gjson.Result) bool {
				appendOpenAIChatTool(spans, tool, roles)
				return true
			})
		}
		format := root.Get("response_format")
		if format.Get("type").Str == "json_schema" {
			jsonSchema := format.Get("json_schema")
			appendStringSpan(spans, jsonSchema.Get("description"), "developer", roles)
			appendJSONSchemaDescriptions(spans, jsonSchema.Get("schema"), "developer", roles)
		}
	}
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

func collectOpenAIResponsesTool(spans *[]textSpan, tool gjson.Result, role string, roles scopeSet) {
	if !tool.IsObject() {
		return
	}
	if tool.Get("type").Str == "shell" {
		environment := tool.Get("environment")
		if environment.Get("type").Str != "local" || !roles.has(role) {
			return
		}
		skills := environment.Get("skills")
		if skills.IsArray() {
			skills.ForEach(func(_, skill gjson.Result) bool {
				appendStringSpan(spans, skill.Get("description"), role, roles)
				return true
			})
		}
		return
	}
	if !roles.has(role) {
		return
	}
	switch tool.Get("type").Str {
	case "function":
		appendStringSpan(spans, tool.Get("description"), role, roles)
		appendJSONSchemaDescriptions(spans, tool.Get("parameters"), role, roles)
		appendJSONSchemaDescriptions(spans, tool.Get("output_schema"), role, roles)
	case "custom":
		appendStringSpan(spans, tool.Get("description"), role, roles)
	case "namespace":
		appendStringSpan(spans, tool.Get("description"), role, roles)
		children := tool.Get("tools")
		if children.IsArray() {
			children.ForEach(func(_, child gjson.Result) bool {
				collectOpenAIResponsesTool(spans, child, role, roles)
				return true
			})
		}
	case "tool_search":
		appendStringSpan(spans, tool.Get("description"), role, roles)
		appendJSONSchemaDescriptions(spans, tool.Get("parameters"), role, roles)
	case "mcp":
		appendStringSpan(spans, tool.Get("server_description"), role, roles)
	}
}

func openAIAdditionalToolsRole(item gjson.Result) (string, bool) {
	role := item.Get("role")
	if role.Type != gjson.String {
		return "", false
	}
	switch role.Str {
	case "system", "developer", "user", "assistant", "tool":
		return role.Str, true
	default:
		return "", false
	}
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
			errorValue := item.Get("error")
			if errorValue.IsObject() {
				switch errorValue.Get("type").Str {
				case "mcp_protocol_error", "http_error":
					appendStringSpan(spans, errorValue.Get("message"), "tool", roles)
				}
			}
		}
		return true
	case "mcp_list_tools":
		if roles.has("tool") {
			appendStringSpan(spans, item.Get("error"), "tool", roles)
			tools := item.Get("tools")
			if tools.IsArray() {
				tools.ForEach(func(_, tool gjson.Result) bool {
					appendStringSpan(spans, tool.Get("description"), "tool", roles)
					appendJSONSchemaDescriptions(spans, tool.Get("input_schema"), "tool", roles)
					return true
				})
			}
		}
		return true
	case "additional_tools":
		role, ok := openAIAdditionalToolsRole(item)
		if !ok {
			return true
		}
		tools := item.Get("tools")
		if tools.IsArray() {
			tools.ForEach(func(_, tool gjson.Result) bool {
				collectOpenAIResponsesTool(spans, tool, role, roles)
				return true
			})
		}
		return true
	case "tool_search_output":
		if roles.has("tool") {
			tools := item.Get("tools")
			if tools.IsArray() {
				tools.ForEach(func(_, tool gjson.Result) bool {
					collectOpenAIResponsesTool(spans, tool, "tool", roles)
					return true
				})
			}
		}
		return true
	case "program_output":
		if roles.has("tool") {
			appendStringSpan(spans, item.Get("result"), "tool", roles)
		}
		return true
	case "file_search_call":
		if !roles.has("tool") {
			return true
		}
		results := item.Get("results")
		if results.IsArray() {
			results.ForEach(func(_, result gjson.Result) bool {
				if result.IsObject() {
					appendStringSpan(spans, result.Get("text"), "tool", roles)
				}
				return true
			})
		}
		return true
	case "code_interpreter_call":
		if !roles.has("tool") {
			return true
		}
		outputs := item.Get("outputs")
		if outputs.IsArray() {
			outputs.ForEach(func(_, output gjson.Result) bool {
				if output.IsObject() && output.Get("type").Str == "logs" {
					appendStringSpan(spans, output.Get("logs"), "tool", roles)
				}
				return true
			})
		}
		return true
	case "mcp_approval_response":
		if roles.has("user") {
			appendStringSpan(spans, item.Get("reason"), "user", roles)
		}
		return true
	default:
		return false
	}
}

func collectOpenAIResponsesPromptVariables(spans *[]textSpan, root gjson.Result, roles scopeSet) {
	if !roles.has("user") {
		return
	}
	variables := root.Get("prompt.variables")
	if !variables.IsObject() {
		return
	}
	variables.ForEach(func(_, value gjson.Result) bool {
		appendStringSpan(spans, value, "user", roles)
		if value.IsObject() && value.Get("type").Str == "input_text" {
			appendStringSpan(spans, value.Get("text"), "user", roles)
		}
		if value.IsArray() {
			value.ForEach(func(_, part gjson.Result) bool {
				if part.Get("type").Str == "input_text" {
					appendStringSpan(spans, part.Get("text"), "user", roles)
				}
				return true
			})
		}
		return true
	})
}

func collectOpenAIResponses(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	if roles.has("system") {
		appendStringSpan(spans, root.Get("instructions"), "system", roles)
	}
	collectOpenAIResponsesPromptVariables(spans, root, roles)
	if roles.has("developer") {
		format := root.Get("text.format")
		if format.Get("type").Str == "json_schema" {
			appendStringSpan(spans, format.Get("description"), "developer", roles)
			appendJSONSchemaDescriptions(spans, format.Get("schema"), "developer", roles)
		}
	}
	if roles.has("developer") || roles.has("user") {
		tools := root.Get("tools")
		if tools.IsArray() {
			tools.ForEach(func(_, tool gjson.Result) bool {
				role := "developer"
				if tool.Get("type").Str == "shell" {
					role = "user"
				}
				collectOpenAIResponsesTool(spans, tool, role, roles)
				return true
			})
		}
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
