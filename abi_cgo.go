//go:build cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
void cliproxyPluginFree(void*, size_t);
void cliproxyPluginShutdown(void);

static void cliproxy_set_plugin_api(cliproxy_plugin_api* plugin, uint32_t abi_version) {
	plugin->abi_version = abi_version;
	plugin->call = (cliproxy_plugin_call_fn)cliproxyPluginCall;
	plugin->free_buffer = cliproxyPluginFree;
	plugin->shutdown = cliproxyPluginShutdown;
}

static int cliproxy_host_call(cliproxy_host_api* host, const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	return host->call(host->host_ctx, method, request, request_len, response);
}

static void cliproxy_host_free(cliproxy_host_api* host, void* ptr, size_t len) {
	host->free_buffer(ptr, len);
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

type pluginCallBuffer = C.cliproxy_buffer
type pluginCallChar = C.char
type pluginCallSize = C.size_t

func shouldCopyPluginRequest(method string) bool {
	return method != pluginabi.MethodRequestInterceptAfter
}

func checkedCIntLength(length uint64) (int, bool) {
	const max = uint64(^uint32(0) >> 1)
	if length > max {
		return 0, false
	}
	return int(length), true
}

func validPluginRequest(ptr unsafe.Pointer, length uint64) bool {
	return ptr != nil || length == 0
}

func copyPluginRequest(ptr unsafe.Pointer, length uint64) ([]byte, error) {
	if _, err := borrowedRequest(ptr, length); err != nil {
		return nil, err
	}
	if length == 0 {
		return nil, nil
	}
	requestLen, ok := checkedCIntLength(length)
	if !ok {
		return nil, fmt.Errorf("request too large: %d", length)
	}
	return C.GoBytes(ptr, C.int(requestLen)), nil
}

func copyHostResponse(ptr unsafe.Pointer, length uint64) ([]byte, error) {
	if length == 0 {
		return nil, nil
	}
	if ptr == nil {
		return nil, fmt.Errorf("host callback response pointer is nil with length %d", length)
	}
	responseLen, ok := checkedCIntLength(length)
	if !ok {
		return nil, fmt.Errorf("host callback response too large: %d", length)
	}
	return C.GoBytes(ptr, C.int(responseLen)), nil
}

// borrowedRequest returns host-owned input valid only during the synchronous ABI callback.
func borrowedRequest(ptr unsafe.Pointer, length uint64) ([]byte, error) {
	if length == 0 {
		return nil, nil
	}
	if length > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("request too large: %d", length)
	}
	if ptr == nil {
		return nil, fmt.Errorf("request pointer is nil with length %d", length)
	}
	return unsafe.Slice((*byte)(ptr), int(length)), nil
}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if host == nil || plugin == nil {
		return 1
	}
	if host.abi_version != C.uint32_t(pluginabi.ABIVersion) {
		return 1
	}
	if host.call == nil || host.free_buffer == nil {
		return 1
	}
	setHostCallback(func(method string, request []byte) ([]byte, error) {
		methodC := C.CString(method)
		defer C.free(unsafe.Pointer(methodC))
		requestPtr := C.CBytes(request)
		defer C.free(requestPtr)
		var response C.cliproxy_buffer
		rc := C.cliproxy_host_call(host, methodC, (*C.uint8_t)(requestPtr), C.size_t(len(request)), &response)
		if response.ptr != nil {
			defer C.cliproxy_host_free(host, response.ptr, response.len)
		}
		if rc != 0 {
			return nil, fmt.Errorf("host callback failed with rc=%d", int(rc))
		}
		responseBytes, err := copyHostResponse(unsafe.Pointer(response.ptr), uint64(response.len))
		if err != nil {
			return nil, err
		}
		return responseBytes, nil
	})
	C.cliproxy_set_plugin_api(plugin, C.uint32_t(pluginabi.ABIVersion))
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response == nil {
		return 1
	}
	response.ptr = nil
	response.len = 0
	if method == nil {
		return 1
	}
	if !validPluginRequest(unsafe.Pointer(request), uint64(requestLen)) {
		return 1
	}
	methodName := C.GoString(method)
	var requestBytes []byte
	if shouldCopyPluginRequest(methodName) {
		copiedRequest, err := copyPluginRequest(unsafe.Pointer(request), uint64(requestLen))
		if err != nil {
			return 1
		}
		requestBytes = copiedRequest
	}
	payload, err := handleMethod(methodName, requestBytes)
	if err != nil {
		payload = errorEnvelope("plugin_error", err.Error())
	}
	if len(payload) == 0 {
		return 0
	}
	ptr := C.malloc(C.size_t(len(payload)))
	if ptr == nil {
		return 1
	}
	copy(unsafe.Slice((*byte)(ptr), len(payload)), payload)
	response.ptr = ptr
	response.len = C.size_t(len(payload))
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, length C.size_t) {
	_ = length
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	setHostCallback(nil)
}
