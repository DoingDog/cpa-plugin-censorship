//go:build integration

package censorshipintegration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/pluginhost"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

var benchmarkDynamicABIResponseSink pluginapi.RequestInterceptResponse

func TestDynamicABIResponseOracle(t *testing.T) {
	input := dynamicABIBody(1024)
	stripped := bytes.ReplaceAll(input, []byte("BLOCKME"), nil)
	mutated := bytes.Clone(input)
	mutated[0] = '!'

	for _, tc := range []struct {
		name        string
		requestBody []byte
		response    pluginapi.RequestInterceptResponse
		wantErr     bool
	}{
		{name: "successful strip", requestBody: bytes.Clone(input), response: pluginapi.RequestInterceptResponse{Body: stripped}},
		{name: "terminated response", requestBody: bytes.Clone(input), response: pluginapi.RequestInterceptResponse{Body: stripped, Terminate: true}, wantErr: true},
		{name: "empty response", requestBody: bytes.Clone(input), wantErr: true},
		{name: "mutated request", requestBody: mutated, response: pluginapi.RequestInterceptResponse{Body: stripped}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDynamicABIResponse("before", input, tc.requestBody, tc.response, stripped)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateDynamicABIResponse() error = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}

func BenchmarkDynamicABIRequestInterceptors(b *testing.B) {
	pluginDir := os.Getenv("CENSORSHIP_PLUGIN_DIR")
	if pluginDir == "" {
		b.Fatal("CENSORSHIP_PLUGIN_DIR is required")
	}
	rawConfig := fmt.Sprintf("plugins:\n  enabled: true\n  dir: %q\n  configs:\n    censorship:\n      enabled: true\n      mode: strip\n      words: [BLOCKME]\n", pluginDir)
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(rawConfig), &cfg); err != nil {
		b.Fatal(err)
	}
	host := pluginhost.New()
	host.ApplyConfig(context.Background(), &cfg)
	b.Cleanup(host.ShutdownAll)
	if !host.PluginRegistered("censorship") {
		b.Fatal("censorship plugin was not registered")
	}

	loader := "unix"
	if runtime.GOOS == "windows" {
		loader = "windows"
	}
	for _, size := range []int{1 << 10, 1 << 20, 20 << 20} {
		input := dynamicABIBody(size)
		stripped := bytes.ReplaceAll(input, []byte("BLOCKME"), nil)
		newRequest := func() pluginapi.RequestInterceptRequest {
			return pluginapi.RequestInterceptRequest{RequestID: "abi-benchmark", SourceFormat: "openai", Body: bytes.Clone(input)}
		}
		cases := []struct {
			phase    string
			expected []byte
			call     func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse
		}{
			{phase: "before", expected: stripped, call: func(request pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse {
				return host.InterceptRequestBeforeAuth(context.Background(), request)
			}},
			{phase: "after", expected: input, call: func(request pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse {
				return host.InterceptRequestAfterAuth(context.Background(), request)
			}},
		}
		for _, tc := range cases {
			tc := tc
			name := fmt.Sprintf("GOOS=%s/loader=%s/phase=%s/calls=1/body=%d", runtime.GOOS, loader, tc.phase, size)
			b.Run(name, func(b *testing.B) {
				request := newRequest()
				response := tc.call(request)
				if err := validateDynamicABIResponse(tc.phase, input, request.Body, response, tc.expected); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs() // Host Go runtime allocations only.
				b.SetBytes(int64(len(input)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					request := newRequest()
					benchmarkDynamicABIResponseSink = tc.call(request)
				}
			})
		}
	}
}

func dynamicABIBody(size int) []byte {
	const prefix = `{"messages":[{"role":"user","content":"`
	const suffix = ` BLOCKME"}]}`
	if size < len(prefix)+len(suffix) {
		panic("dynamic ABI body size too small")
	}
	return []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
}

func validateDynamicABIResponse(phase string, input, requestBody []byte, response pluginapi.RequestInterceptResponse, expected []byte) error {
	if !bytes.Equal(requestBody, input) {
		return fmt.Errorf("%s request body was mutated", phase)
	}
	if response.Terminate {
		return fmt.Errorf("%s interceptor terminated the request", phase)
	}
	if !bytes.Equal(response.Body, expected) {
		return fmt.Errorf("%s response body length = %d, want %d", phase, len(response.Body), len(expected))
	}
	return nil
}
