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
		body := dynamicABIBody(size)
		req := pluginapi.RequestInterceptRequest{RequestID: "abi-benchmark", SourceFormat: "openai", Body: body}
		cases := []struct {
			phase string
			call  func() pluginapi.RequestInterceptResponse
		}{
			{phase: "before", call: func() pluginapi.RequestInterceptResponse {
				return host.InterceptRequestBeforeAuth(context.Background(), req)
			}},
			{phase: "after", call: func() pluginapi.RequestInterceptResponse {
				return host.InterceptRequestAfterAuth(context.Background(), req)
			}},
		}
		for _, tc := range cases {
			tc := tc
			name := fmt.Sprintf("GOOS=%s/loader=%s/phase=%s/calls=1/body=%d", runtime.GOOS, loader, tc.phase, size)
			b.Run(name, func(b *testing.B) {
				resp := tc.call()
				switch tc.phase {
				case "before":
					if bytes.Contains(resp.Body, []byte("BLOCKME")) {
						b.Fatalf("%s oracle failed: body length %d", tc.phase, len(resp.Body))
					}
				case "after":
					if !bytes.Equal(resp.Body, body) {
						b.Fatalf("%s oracle failed: body length %d", tc.phase, len(resp.Body))
					}
				}
				b.ReportAllocs() // Host Go runtime allocations only.
				b.SetBytes(int64(len(body)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					benchmarkDynamicABIResponseSink = tc.call()
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
