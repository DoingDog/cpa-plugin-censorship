//go:build cgo

package main

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestCheckedCIntLength(t *testing.T) {
	max := uint64(^uint32(0) >> 1)
	for _, tc := range []struct {
		name string
		in   uint64
		want int
		ok   bool
	}{
		{name: "zero", in: 0, want: 0, ok: true},
		{name: "max", in: max, want: int(max), ok: true},
		{name: "overflow", in: max + 1, want: 0, ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := checkedCIntLength(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("checkedCIntLength(%d) = %d, %t; want %d, %t", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

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
