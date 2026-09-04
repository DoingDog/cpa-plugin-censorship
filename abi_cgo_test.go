//go:build cgo

package main

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestShouldCopyPluginRequest(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{pluginabi.MethodRequestInterceptAfter, false},
		{pluginabi.MethodRequestInterceptBefore, true},
		{pluginabi.MethodPluginRegister, true},
		{pluginabi.MethodPluginReconfigure, true},
		{"unknown_method", true},
	}

	for _, test := range tests {
		if got := shouldCopyPluginRequest(test.method); got != test.want {
			t.Errorf("shouldCopyPluginRequest(%q) = %t, want %t", test.method, got, test.want)
		}
	}
}
