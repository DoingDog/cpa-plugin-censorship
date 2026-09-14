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
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"gopkg.in/yaml.v3"
)

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

const dynamicABIModelFilterConfig = `      filter_mode: include
      filter:
        models: [target-model]
      words:
        strip: [BLOCKME]
`

func withDynamicABIHost(t testing.TB, censorshipConfig string, run func(
	func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse,
	func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse,
)) {
	t.Helper()
	pluginDir := os.Getenv("CENSORSHIP_PLUGIN_DIR")
	if pluginDir == "" {
		t.Fatal("CENSORSHIP_PLUGIN_DIR is required")
	}
	rawConfig := fmt.Sprintf("plugins:\n  enabled: true\n  dir: %q\n  configs:\n    censorship:\n      enabled: true\n%s", pluginDir, censorshipConfig)
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(rawConfig), &cfg); err != nil {
		t.Fatal(err)
	}
	host := pluginhost.New()
	host.ApplyConfig(context.Background(), &cfg)
	t.Cleanup(host.ShutdownAll)
	if !host.PluginRegistered("censorship") {
		t.Fatal("censorship plugin was not registered")
	}
	run(
		func(request pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse {
			return host.InterceptRequestBeforeAuth(context.Background(), request)
		},
		func(request pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse {
			return host.InterceptRequestAfterAuth(context.Background(), request)
		},
	)
}

func TestDynamicABIActiveAfter(t *testing.T) {
	input := dynamicABIBody(1024)
	stripped := bytes.ReplaceAll(input, []byte("BLOCKME"), nil)
	withDynamicABIHost(t, dynamicABIModelFilterConfig, func(before, after func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse) {
		request := pluginapi.RequestInterceptRequest{
			RequestID:      "dynamic-abi-after",
			SourceFormat:   "openai",
			Model:          "target-model",
			RequestedModel: "requested-decoy",
			Metadata:       map[string]any{executor.SelectedAuthMetadataKey: "auth-1"},
			Body:           bytes.Clone(input),
		}
		if err := validateDynamicABIResponse("before", input, request.Body, before(request), input); err != nil {
			t.Fatal(err)
		}
		response := after(request)
		if err := validateDynamicABIResponse("after", input, request.Body, response, stripped); err != nil {
			t.Fatal(err)
		}
	})
}

func BenchmarkDynamicABIRequestInterceptors(b *testing.B) {
	withDynamicABIHost(b, dynamicABIModelFilterConfig, func(before, after func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse) {
		loader := "unix"
		if runtime.GOOS == "windows" {
			loader = "windows"
		}
		for _, size := range []int{1 << 10, 1 << 20, 20 << 20} {
			input := dynamicABIBody(size)
			stripped := bytes.ReplaceAll(input, []byte("BLOCKME"), nil)
			cases := []struct {
				phase    string
				expected []byte
				call     func(pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse
			}{
				{phase: "before", expected: input, call: before},
				{phase: "after", expected: stripped, call: after},
			}
			for _, tc := range cases {
				tc := tc
				name := fmt.Sprintf("GOOS=%s/loader=%s/phase=%s/calls=1/body=%d", runtime.GOOS, loader, tc.phase, size)
				newRequest := func() pluginapi.RequestInterceptRequest {
					request := pluginapi.RequestInterceptRequest{
						RequestID:      "abi-benchmark",
						SourceFormat:   "openai",
						Model:          "target-model",
						RequestedModel: "requested-decoy",
						Body:           bytes.Clone(input),
					}
					if tc.phase == "after" {
						request.Metadata = map[string]any{executor.SelectedAuthMetadataKey: "auth-1"}
					}
					return request
				}
				validate := func(t testing.TB, request pluginapi.RequestInterceptRequest, response pluginapi.RequestInterceptResponse) {
					t.Helper()
					if err := validateDynamicABIResponse(tc.phase, input, request.Body, response, tc.expected); err != nil {
						t.Fatal(err)
					}
				}
				b.Run(name+"/serial", func(b *testing.B) {
					request := newRequest()
					response := tc.call(request)
					validate(b, request, response)
					b.ReportAllocs() // Host Go runtime allocations only.
					b.SetBytes(int64(len(input)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						response = tc.call(request)
					}
					b.StopTimer()
					validate(b, request, response)
				})
				b.Run(name+"/parallel", func(b *testing.B) {
					request := newRequest()
					response := tc.call(request)
					validate(b, request, response)
					b.ReportAllocs() // Host Go runtime allocations only.
					b.SetBytes(int64(len(input)))
					b.ResetTimer()
					b.RunParallel(func(pb *testing.PB) {
						var response pluginapi.RequestInterceptResponse
						ran := false
						for pb.Next() {
							response = tc.call(request)
							ran = true
						}
						if !ran {
							return
						}
						if err := validateDynamicABIResponse(tc.phase, input, request.Body, response, tc.expected); err != nil {
							b.Error(err)
						}
					})
					b.StopTimer()
				})
			}
		}
	})
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
