//go:build cgo

package main

import (
	"bytes"
	"encoding/json"
	"reflect"
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
		{pluginabi.MethodRequestInterceptAfter, true},
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

func TestValidPluginRequest(t *testing.T) {
	input := []byte{0x7f}
	for _, tc := range []struct {
		name   string
		ptr    unsafe.Pointer
		length uint64
		want   bool
	}{
		{name: "nil zero", ptr: nil, length: 0, want: true},
		{name: "nil one", ptr: nil, length: 1, want: false},
		{name: "non-nil one", ptr: unsafe.Pointer(&input[0]), length: 1, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validPluginRequest(tc.ptr, tc.length); got != tc.want {
				t.Fatalf("validPluginRequest(%v, %d) = %t, want %t", tc.ptr, tc.length, got, tc.want)
			}
		})
	}
}

func TestCliproxyPluginCallRejectsNilNonzeroAfterAuthRequest(t *testing.T) {
	method := append([]byte(pluginabi.MethodRequestInterceptAfter), 0)
	var response pluginCallBuffer
	if rc := cliproxyPluginCall((*pluginCallChar)(unsafe.Pointer(&method[0])), nil, pluginCallSize(1), &response); rc != 1 {
		t.Fatalf("nil after-auth request with nonzero length returned %d, want 1", rc)
	}
	if response.ptr != nil || response.len != 0 {
		t.Fatalf("rejected request response = (%v, %d), want (nil, 0)", response.ptr, response.len)
	}
}

func TestCliproxyPluginCallProcessesAfterAuthRequest(t *testing.T) {
	registerConfig(t, "filter_mode: include\nfilter:\n  models: [target-model]\nwords:\n  strip: [BLOCKME]\n")
	request, err := json.Marshal(pluginapi.RequestInterceptRequest{
		RequestID:      "after-auth-exported-call",
		SourceFormat:   "openai",
		Model:          "target-model",
		RequestedModel: "requested-decoy",
		Metadata:       selectedAuthMetadata(),
		Body:           []byte(`{"messages":[{"role":"user","content":"before BLOCKME after"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	method := append([]byte(pluginabi.MethodRequestInterceptAfter), 0)
	var response pluginCallBuffer
	call := reflect.ValueOf(cliproxyPluginCall)
	requestArg := reflect.NewAt(call.Type().In(1).Elem(), unsafe.Pointer(&request[0]))
	results := call.Call([]reflect.Value{
		reflect.ValueOf((*pluginCallChar)(unsafe.Pointer(&method[0]))),
		requestArg,
		reflect.ValueOf(pluginCallSize(len(request))),
		reflect.ValueOf(&response),
	})
	if rc := results[0].Int(); rc != 0 {
		t.Fatalf("cliproxyPluginCall() = %d, want 0", rc)
	}
	if response.ptr == nil || response.len == 0 {
		t.Fatalf("cliproxyPluginCall() response = (%v, %d), want non-empty buffer", response.ptr, response.len)
	}
	defer cliproxyPluginFree(unsafe.Pointer(response.ptr), response.len)

	for i := range request {
		request[i] = 0xa5
	}
	// The response buffer is independent of host request memory; this does not prove input copying.
	responseRaw := unsafe.Slice((*byte)(unsafe.Pointer(response.ptr)), int(response.len))
	var env pluginabi.Envelope
	decodeEnvelope(t, responseRaw, &env)
	got := decodeResult[pluginapi.RequestInterceptResponse](t, env)
	if got.Terminate || string(got.Body) != `{"messages":[{"role":"user","content":"before  after"}]}` {
		t.Fatalf("response = %#v, want stripped body", got)
	}
}

func TestCopyPluginRequest(t *testing.T) {
	got, err := copyPluginRequest(nil, 0)
	if err != nil || got != nil {
		t.Fatalf("empty request = %v, %v", got, err)
	}
	if _, err := copyPluginRequest(nil, 1); err == nil {
		t.Fatal("nil request with nonzero length succeeded")
	}
	input := []byte{0x7f}
	got, err = copyPluginRequest(unsafe.Pointer(&input[0]), 1)
	if err != nil || !bytes.Equal(got, input) {
		t.Fatalf("copied request = %x, %v", got, err)
	}
	input[0] = 0
	if got[0] != 0x7f {
		t.Fatal("copied request aliases caller memory")
	}
}

func TestCopyHostResponseRejectsNilNonzeroLength(t *testing.T) {
	if _, err := copyHostResponse(nil, 1); err == nil {
		t.Fatal("nil host response with nonzero length succeeded")
	}
	got, err := copyHostResponse(nil, 0)
	if err != nil || got != nil {
		t.Fatalf("empty response = %v, %v", got, err)
	}
	input := []byte{0x7f}
	got, err = copyHostResponse(unsafe.Pointer(&input[0]), 1)
	if err != nil || !bytes.Equal(got, input) {
		t.Fatalf("copied response = %x, %v", got, err)
	}
	input[0] = 0
	if got[0] != 0x7f {
		t.Fatal("copied response aliases host memory")
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
			Body:         []byte(`{"messages":[{"role":"user","content":""}]}`),
			ClearHeaders: []string{"Content-Encoding", "Content-Length", "Transfer-Encoding"},
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

func BenchmarkPluginRequestOwnership(b *testing.B) {
	for _, size := range []struct {
		name string
		len  int
	}{
		{name: "1KiB", len: 1 << 10},
		{name: "1MiB", len: 1 << 20},
		{name: "20MiB", len: 20 << 20},
	} {
		input := bytes.Repeat([]byte{0xa5}, size.len)
		input[0] = 0x7f
		input[len(input)-1] = 0x2a
		requestPtr := unsafe.Pointer(&input[0])

		for _, impl := range []struct {
			name    string
			request func(unsafe.Pointer, uint64) ([]byte, error)
		}{
			{name: "copy", request: copyPluginRequest},
			{name: "borrow", request: borrowedRequest},
		} {
			impl := impl
			b.Run("impl="+impl.name+"/body="+size.name+"/serial", func(b *testing.B) {
				var result []byte
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					var err error
					result, err = impl.request(requestPtr, uint64(len(input)))
					if err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if len(result) != len(input) || result[0] != input[0] || result[len(result)-1] != input[len(input)-1] {
					b.Fatal("request result does not match input")
				}
			})
			b.Run("impl="+impl.name+"/body="+size.name+"/parallel", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(input)))
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					var result []byte
					ran := false
					for pb.Next() {
						var err error
						result, err = impl.request(requestPtr, uint64(len(input)))
						if err != nil {
							b.Error(err)
							return
						}
						ran = true
					}
					if !ran {
						return
					}
					if len(result) != len(input) || result[0] != input[0] || result[len(result)-1] != input[len(input)-1] {
						b.Error("request result does not match input")
					}
				})
				b.StopTimer()
			})
		}
	}
}
