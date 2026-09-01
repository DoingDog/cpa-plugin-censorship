package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func main() {}

var pluginVersion = "0.0.0-dev"

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
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
			ConfigFields:     []pluginapi.ConfigField{},
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
		_, _ = callHost(pluginabi.MethodHostLog, hostLogRequest{
			Level:   "error",
			Message: "censorship plugin reconfigure rejected",
			Fields:  map[string]any{"error": err.Error()},
		})
		return okEnvelope(pluginRegistration())
	}
	installSnapshot(candidate)
	return okEnvelope(pluginRegistration())
}

func parseLifecycleSnapshot(raw []byte) (*configSnapshot, error) {
	var lifecycle lifecycleRequest
	if err := json.Unmarshal(raw, &lifecycle); err != nil {
		return nil, err
	}
	return parseConfigYAML(lifecycle.ConfigYAML)
}

func interceptBeforeAuth(raw []byte) ([]byte, error) {
	var request pluginapi.RequestInterceptRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	cfg := loadedSnapshot()
	if !knownSourceFormat(request.SourceFormat) {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
	result, err := transformRequest(request.Body, request.SourceFormat, cfg)
	if err != nil {
		return nil, err
	}
	switch {
	case result.Invalid:
		return terminatedRequest(censorshipError{
			Type:    "invalid_request_error",
			Code:    "censorship_invalid_request",
			Message: "request body must be a JSON object",
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
		return okEnvelope(pluginapi.RequestInterceptResponse{Body: result.Body})
	default:
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}
}

func interceptAfterAuth([]byte) ([]byte, error) {
	return okEnvelope(pluginapi.RequestInterceptResponse{})
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

func okEnvelope(value any) ([]byte, error) {
	result, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(pluginabi.Envelope{OK: true, Result: result})
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
