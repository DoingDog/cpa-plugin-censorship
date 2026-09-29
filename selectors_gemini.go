package main

import "github.com/tidwall/gjson"

func appendGeminiTextPart(spans *[]textSpan, part gjson.Result, role string, roles scopeSet) {
	text, allowed, signatureBound := scanTextPart(part, false)
	if !allowed {
		return
	}
	before := len(*spans)
	appendStringSpan(spans, text, role, roles)
	if signatureBound && len(*spans) != before {
		(*spans)[len(*spans)-1].RequiresUnmodified = true
	}
}

func collectGemini(root gjson.Result, roles scopeSet, spans *[]textSpan) {
	collectParts := func(content gjson.Result, role string) {
		if !roles.has(role) {
			return
		}
		parts := content.Get("parts")
		if !parts.IsArray() {
			return
		}
		parts.ForEach(func(_, part gjson.Result) bool {
			appendGeminiTextPart(spans, part, role, roles)
			return true
		})
	}

	if roles.has("developer") {
		tools := root.Get("tools")
		if tools.IsArray() {
			tools.ForEach(func(_, tool gjson.Result) bool {
				if !tool.IsObject() {
					return true
				}
				for _, declarationsKey := range []string{"functionDeclarations", "function_declarations"} {
					declarations := tool.Get(declarationsKey)
					if !declarations.IsArray() {
						continue
					}
					declarations.ForEach(func(_, declaration gjson.Result) bool {
						if !declaration.IsObject() {
							return true
						}
						appendStringSpan(spans, declaration.Get("description"), "developer", roles)
						for _, key := range []string{"parameters", "parametersJsonSchema", "parameters_json_schema", "response", "responseJsonSchema", "response_json_schema"} {
							appendJSONSchemaDescriptions(spans, declaration.Get(key), "developer", roles)
						}
						return true
					})
				}
				return true
			})
		}
		for _, key := range []string{"responseSchema", "responseJsonSchema", "responseFormat.text.schema"} {
			appendJSONSchemaDescriptions(spans, root.Get("generationConfig."+key), "developer", roles)
		}
	}
	if roles.has("system") {
		collectParts(root.Get("systemInstruction"), "system")
		collectParts(root.Get("system_instruction"), "system")
	}
	if !roles.has("user") && !roles.has("assistant") {
		return
	}

	contents := root.Get("contents")
	if !contents.IsArray() {
		return
	}
	previousRole := ""
	contents.ForEach(func(_, content gjson.Result) bool {
		role := content.Get("role")
		var effectiveRole string
		switch {
		case !role.Exists() || role.Type == gjson.Null || role.String() == "":
			if geminiHasFunctionResponse(content) {
				previousRole = "user"
			} else {
				previousRole = nextGeminiRole(previousRole)
			}
			if previousRole == "user" {
				effectiveRole = "user"
			} else {
				effectiveRole = "assistant"
			}
		case role.Type == gjson.String && role.Str == "user":
			previousRole = "user"
			effectiveRole = "user"
		case role.Type == gjson.String && role.Str == "model":
			previousRole = "model"
			effectiveRole = "assistant"
		default:
			if geminiHasFunctionResponse(content) {
				previousRole = "user"
			} else {
				previousRole = nextGeminiRole(previousRole)
			}
			return true
		}
		collectParts(content, effectiveRole)
		return true
	})
}

func geminiHasFunctionResponse(content gjson.Result) bool {
	parts := content.Get("parts")
	found := false
	if parts.IsArray() {
		parts.ForEach(func(_, part gjson.Result) bool {
			if part.Get("functionResponse").Exists() || part.Get("function_response").Exists() {
				found = true
				return false
			}
			return true
		})
	}
	return found
}

func nextGeminiRole(previousRole string) string {
	if previousRole == "" || previousRole == "model" {
		return "user"
	}
	return "model"
}
