package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func main() {}

var pluginVersion = "0.0.0-dev"

type lifecycleRequest struct {
	ConfigYAML    *[]byte `json:"config_yaml"`
	SchemaVersion uint32  `json:"schema_version"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationCapabilities struct {
	RequestInterceptor        bool `json:"request_interceptor"`
	RequestLifecyclePlugin    bool `json:"request_lifecycle_plugin"`
	ResponseInterceptor       bool `json:"response_interceptor"`
	StreamChunkInterceptor    bool `json:"response_stream_interceptor"`
	WebSocketResponseObserver bool `json:"websocket_response_observer"`
	ManagementAPI             bool `json:"management_api"`
	ModelRouter               bool `json:"model_router"`
	Executor                  bool `json:"executor"`
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "censorship",
			Version:          pluginVersion,
			Author:           "DoingDog",
			GitHubRepository: "https://github.com/DoingDog/cpa-plugin-censorship",
			Logo:             "https://raw.githubusercontent.com/DoingDog/cpa-plugin-censorship/main/logo.png",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "ignore_case", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Match terms with Go unicode.SimpleFold equivalence (default false)."},
				{Name: "words", Type: pluginapi.ConfigFieldTypeObject, Description: "Optional block, strip, and obfs arrays; an empty object has no rules."},
				{Name: "scope", Type: pluginapi.ConfigFieldTypeObject, Description: "Optional object with formats and roles arrays; defaults to all supported formats and system, developer, and user roles."},
				{Name: "obfs", Type: pluginapi.ConfigFieldTypeObject, Description: "Obfuscation object whose char is U+200B or U+2060; used by obfs rules (default U+200B)."},
				{Name: "filter_mode", Type: pluginapi.ConfigFieldTypeEnum, Description: "Exclude matching requests from censorship, or include only matching requests (default exclude).", EnumValues: []string{"exclude", "include"}},
				{Name: "filter_logic", Type: pluginapi.ConfigFieldTypeEnum, Description: "Combine non-empty filter api-keys and models with or or and (default or); one non-empty list is used alone.", EnumValues: []string{"or", "and"}},
				{Name: "filter", Type: pluginapi.ConfigFieldTypeObject, Description: "Optional object with api-keys and models pattern arrays; * matches zero or more Unicode scalars and ? matches one. Empty arrays or an empty object disable request filtering. API-key patterns require authenticated caller_scope binding and are not masked by the standard panel."},
			},
		},
		Capabilities: registrationCapabilities{RequestInterceptor: true},
	}
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister:
		return handlePluginRegister(request)
	case pluginabi.MethodPluginReconfigure:
		return handlePluginReconfigure(request)
	case pluginabi.MethodRequestInterceptBefore:
		return interceptBeforeAuth(request)
	case pluginabi.MethodRequestInterceptAfter:
		return interceptAfterAuth(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func handlePluginRegister(raw []byte) ([]byte, error) {
	candidate, err := parseLifecycleSnapshot(raw)
	if err != nil {
		return errorEnvelope("plugin_error", err.Error()), nil
	}
	installSnapshot(candidate)
	return okEnvelope(pluginRegistration())
}

func handlePluginReconfigure(raw []byte) ([]byte, error) {
	candidate, err := parseLifecycleSnapshot(raw)
	if err != nil {
		logReconfigureRejected(err.Error())
		return okEnvelope(pluginRegistration())
	}
	current := loadedSnapshot()
	oldHasModels := current != nil && len(current.Filter.Models) != 0
	newHasModels := len(candidate.Filter.Models) != 0
	if oldHasModels != newHasModels {
		logReconfigureRejected("filter.models phase change requires restart")
		return okEnvelope(pluginRegistration())
	}
	installSnapshot(candidate)
	return okEnvelope(pluginRegistration())
}

func logReconfigureRejected(errorText string) {
	_, _ = callHost(pluginabi.MethodHostLog, hostLogRequest{
		Level:   "error",
		Message: "censorship plugin reconfigure rejected",
		Fields:  map[string]any{"error": errorText},
	})
}

func parseLifecycleSnapshot(raw []byte) (*configSnapshot, error) {
	var lifecycle *lifecycleRequest
	if err := json.Unmarshal(raw, &lifecycle); err != nil {
		return nil, err
	}
	if lifecycle == nil || lifecycle.ConfigYAML == nil {
		return nil, fmt.Errorf("config_yaml must be present and non-null")
	}
	return parseConfigYAML(*lifecycle.ConfigYAML)
}

func interceptBeforeAuth(raw []byte) ([]byte, error) {
	cfg := loadedSnapshot()
	if cfg == nil || len(cfg.Rules) == 0 || len(cfg.Filter.Models) != 0 {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
	return interceptRequest(raw, cfg, false)
}

func interceptAfterAuth(raw []byte) ([]byte, error) {
	cfg := loadedSnapshot()
	if cfg == nil || len(cfg.Rules) == 0 || len(cfg.Filter.Models) == 0 {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
	return interceptRequest(raw, cfg, true)
}

func hasSelectedAuth(metadata map[string]any) bool {
	for _, key := range []string{
		cliproxyexecutor.SelectedAuthMetadataKey,
		cliproxyexecutor.SelectedAuthIndexMetadataKey,
	} {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func interceptRequest(raw []byte, cfg *configSnapshot, requireSelectedAuth bool) ([]byte, error) {
	var request pluginapi.RequestInterceptRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if !knownSourceFormat(request.SourceFormat) {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
	if requireSelectedAuth && !hasSelectedAuth(request.Metadata) {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
	if !cfg.Filter.shouldProcess(&request) {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
	result, err := transformRequest(request.Body, request.SourceFormat, cfg)
	if err != nil {
		return nil, err
	}
	switch {
	case result.Invalid:
		message := result.InvalidMessage
		if message == "" {
			message = "request body must be a JSON object"
		}
		return terminatedRequest(censorshipError{
			Type:    "invalid_request_error",
			Code:    "censorship_invalid_request",
			Message: message,
		})
	case result.Blocked != nil:
		return terminatedRequest(censorshipError{
			Type:    "invalid_request_error",
			Code:    "censorship_blocked",
			Message: "request blocked by censorship rule",
			Term:    result.Blocked.Term,
			Role:    result.Blocked.Role,
		})
	case len(result.Body) != 0:
		return okEnvelope(pluginapi.RequestInterceptResponse{
			Body:         result.Body,
			ClearHeaders: []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"},
		})
	default:
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
}

type censorshipErrorBody struct {
	Error censorshipError `json:"error"`
}

type censorshipError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Term    string `json:"term,omitempty"`
	Role    string `json:"role,omitempty"`
}

func terminatedRequest(detail censorshipError) ([]byte, error) {
	body, err := json.Marshal(censorshipErrorBody{Error: detail})
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return okEnvelope(pluginapi.RequestInterceptResponse{
		Terminate:       true,
		StatusCode:      http.StatusBadRequest,
		ResponseHeaders: headers,
		ResponseBody:    body,
	})
}

type successEnvelope struct {
	OK     bool `json:"ok"`
	Result any  `json:"result"`
}

func okEnvelope(value any) ([]byte, error) {
	return json.Marshal(successEnvelope{OK: true, Result: value})
}

func errorEnvelope(code, message string) []byte {
	raw, err := json.Marshal(pluginabi.Envelope{
		OK:    false,
		Error: &pluginabi.Error{Code: code, Message: message},
	})
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"plugin_error","message":"failed to encode error envelope"}}`)
	}
	return raw
}

type hostLogRequest struct {
	Level   string         `json:"level,omitempty"`
	Message string         `json:"message,omitempty"`
	Fields  map[string]any `json:"fields,omitempty"`
}

type hostCallback func(method string, request []byte) ([]byte, error)

var (
	hostAPIMu      sync.RWMutex
	hostCallbackFn hostCallback
)

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	hostAPIMu.RLock()
	callback := hostCallbackFn
	hostAPIMu.RUnlock()
	if callback == nil {
		return nil, fmt.Errorf("host API not initialized")
	}
	response, err := callback(method, rawPayload)
	if err != nil {
		return nil, err
	}
	var envelope pluginabi.Envelope
	if err := json.Unmarshal(response, &envelope); err != nil {
		return nil, fmt.Errorf("decode host envelope: %w", err)
	}
	if !envelope.OK {
		if envelope.Error == nil {
			return nil, fmt.Errorf("host callback %s failed", method)
		}
		return nil, fmt.Errorf("host callback %s failed: %s", method, envelope.Error.Message)
	}
	return append(json.RawMessage(nil), envelope.Result...), nil
}

func setHostCallbackForTest(callback hostCallback) {
	hostAPIMu.Lock()
	hostCallbackFn = callback
	hostAPIMu.Unlock()
}

func setHostCallback(callback hostCallback) {
	setHostCallbackForTest(callback)
}
