//go:build cgo

package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
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

func TestBorrowedRequest(t *testing.T) {
	t.Run("zero length", func(t *testing.T) {
		got, err := borrowedRequest(nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("zero-length request = %v, want nil", got)
		}
	})

	t.Run("one byte", func(t *testing.T) {
		input := []byte{0x7f}
		got, err := borrowedRequest(unsafe.Pointer(&input[0]), 1)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, input) {
			t.Fatalf("one-byte request = %x, want 7f", got)
		}
	})

	t.Run("nil pointer with length", func(t *testing.T) {
		if _, err := borrowedRequest(nil, 1); err == nil {
			t.Fatal("nil pointer with nonzero length succeeded")
		}
	})

	t.Run("length exceeds Go int", func(t *testing.T) {
		maxInt := uint64(^uint(0) >> 1)
		if maxInt == ^uint64(0) {
			t.Skip("cannot represent a length larger than int")
		}
		if _, err := borrowedRequest(nil, maxInt+1); err == nil {
			t.Fatal("length larger than Go int succeeded")
		}
	})
}

func TestBorrowedABIRequestMatchesHandleMethod(t *testing.T) {
	registerConfig(t, "words:\n  strip:\n    - BLOCKME\n")

	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "matching", body: `{"messages":[{"role":"user","content":"BLOCKME"}]}`},
		{name: "nonmatching", body: `{"messages":[{"role":"user","content":"clean"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := json.Marshal(pluginapi.RequestInterceptRequest{
				RequestID:    "borrowed-request-" + tc.name,
				SourceFormat: "openai",
				Body:         []byte(tc.body),
			})
			if err != nil {
				t.Fatal(err)
			}
			want, err := handleMethod(pluginabi.MethodRequestInterceptBefore, request)
			if err != nil {
				t.Fatal(err)
			}
			gotRequest, err := borrowedRequest(unsafe.Pointer(&request[0]), uint64(len(request)))
			if err != nil {
				t.Fatal(err)
			}
			got, err := handleMethod(pluginabi.MethodRequestInterceptBefore, gotRequest)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("borrowed response = %s, want %s", got, want)
			}
		})
	}
}

func TestBorrowedABIResponseSurvivesHostRequestPoison(t *testing.T) {
	registerConfig(t, "words:\n  strip:\n    - BLOCKME\n")
	request, err := json.Marshal(pluginapi.RequestInterceptRequest{
		RequestID:    "response-ownership",
		SourceFormat: "openai",
		Body:         []byte(`{"messages":[{"role":"user","content":"BLOCKME"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	gotRequest, err := borrowedRequest(unsafe.Pointer(&request[0]), uint64(len(request)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := handleMethod(pluginabi.MethodRequestInterceptBefore, gotRequest)
	if err != nil {
		t.Fatal(err)
	}
	for i := range request {
		request[i] = 0xa5
	}
	want, err := json.Marshal(struct {
		OK     bool                               `json:"ok"`
		Result pluginapi.RequestInterceptResponse `json:"result"`
	}{
		OK: true,
		Result: pluginapi.RequestInterceptResponse{
			Body: []byte(`{"messages":[{"role":"user","content":""}]}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("response after host request poison = %s, want %s", got, want)
	}
	if bytes.Contains(got, []byte{0xa5}) {
		t.Fatal("response contains poisoned host request bytes")
	}
}
