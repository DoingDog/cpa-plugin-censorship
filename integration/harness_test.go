//go:build integration

package censorshipintegration

import (
	"bufio"
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const (
	cpaSHA               = "c76dfd4e0edabab9000628b1560ab8ab379eadb8"
	downstreamKey        = "censorship-integration-key"
	modelName            = "censorship-integration-model"
	integrationIOTimeout = 5 * time.Second
)

var integrationHTTPClient = &http.Client{Timeout: integrationIOTimeout}

type cpaPaths struct {
	binary    string
	pluginDir string
}

func resolveCPAPaths(binaryPath, pluginPath string) (cpaPaths, error) {
	return resolveCPAPathsWithBuildInfo(binaryPath, pluginPath, buildinfo.ReadFile)
}

func resolveCPAPathsWithBuildInfo(binaryPath, pluginPath string, readBuildInfo func(string) (*debug.BuildInfo, error)) (cpaPaths, error) {
	if binaryPath == "" {
		return cpaPaths{}, errors.New("CPA_INTEGRATION_BIN is required")
	}
	if pluginPath == "" {
		return cpaPaths{}, errors.New("CENSORSHIP_PLUGIN_DIR is required")
	}

	binary, err := exec.LookPath(binaryPath)
	if err != nil {
		return cpaPaths{}, fmt.Errorf("resolve CPA binary %q: %w", binaryPath, err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return cpaPaths{}, fmt.Errorf("make CPA binary path absolute: %w", err)
	}
	pluginDir, err := filepath.Abs(pluginPath)
	if err != nil {
		return cpaPaths{}, fmt.Errorf("make plugin directory path absolute: %w", err)
	}

	info, err := readBuildInfo(binary)
	if err != nil {
		return cpaPaths{}, fmt.Errorf("read CPA build info %q: %w", binary, err)
	}
	actualRevision := ""
	if info != nil {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				actualRevision = setting.Value
				break
			}
		}
	}
	if actualRevision != cpaSHA {
		return cpaPaths{}, fmt.Errorf("CPA vcs.revision mismatch: expected %q, actual %q", cpaSHA, actualRevision)
	}
	return cpaPaths{binary: binary, pluginDir: pluginDir}, nil
}

func TestResolveCPAPathsNormalizesRelativePaths(t *testing.T) {
	binaryName := "cpa"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binary := filepath.Join(t.TempDir(), binaryName)
	if err := os.WriteFile(binary, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	pluginDir := t.TempDir()
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeBinary, err := filepath.Rel(workingDir, binary)
	if err != nil {
		t.Fatal(err)
	}
	relativePluginDir, err := filepath.Rel(workingDir, pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	readBuildInfo := func(string) (*debug.BuildInfo, error) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: cpaSHA}}}, nil
	}

	absolutePaths, err := resolveCPAPathsWithBuildInfo(binary, pluginDir, readBuildInfo)
	if err != nil {
		t.Fatal(err)
	}
	relativePaths, err := resolveCPAPathsWithBuildInfo(relativeBinary, relativePluginDir, readBuildInfo)
	if err != nil {
		t.Fatal(err)
	}
	if relativePaths != absolutePaths {
		t.Fatalf("relative paths = %#v, absolute paths = %#v", relativePaths, absolutePaths)
	}
	if !filepath.IsAbs(absolutePaths.binary) || !filepath.IsAbs(absolutePaths.pluginDir) {
		t.Fatalf("resolved paths are not absolute: %#v", absolutePaths)
	}
}

func TestRevisionRejectsWrongAndMissingVCSRevision(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pluginDir := t.TempDir()
	for _, tc := range []struct {
		name     string
		settings []debug.BuildSetting
		actual   string
	}{
		{name: "wrong", settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "wrong-revision"}}, actual: "wrong-revision"},
		{name: "missing", actual: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveCPAPathsWithBuildInfo(binary, pluginDir, func(string) (*debug.BuildInfo, error) {
				return &debug.BuildInfo{Settings: tc.settings}, nil
			})
			if err == nil {
				t.Fatal("expected revision mismatch")
			}
			if !strings.Contains(err.Error(), cpaSHA) || !strings.Contains(err.Error(), fmt.Sprintf("%q", tc.actual)) {
				t.Fatalf("mismatch error %q does not include expected %q and actual %q", err, cpaSHA, tc.actual)
			}
		})
	}
}

func TestReadinessUsesAuthenticatedModelsEndpoint(t *testing.T) {
	type readinessRequest struct {
		method        string
		authorization string
	}
	seenRequest := make(chan readinessRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		seenRequest <- readinessRequest{method: r.Method, authorization: r.Header.Get("Authorization")}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logPath := filepath.Join(t.TempDir(), "cpa.log")
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitForCPA(t, &cpaInstance{
		baseURL:  server.URL,
		waitDone: make(chan error),
		logPath:  logPath,
	})
	request := <-seenRequest
	if request.method != http.MethodGet {
		t.Fatalf("method = %q", request.method)
	}
	if request.authorization != "Bearer "+downstreamKey {
		t.Fatalf("Authorization = %q", request.authorization)
	}
}

func TestHTTPClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	started := time.Now()
	_, err := integrationHTTPClient.Get(server.URL)
	if err == nil {
		t.Fatal("request unexpectedly completed")
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("error = %v, want timeout", err)
	}
	if elapsed := time.Since(started); elapsed < 4*time.Second || elapsed > 10*time.Second {
		t.Fatalf("HTTP timeout elapsed = %v", elapsed)
	}
}

func TestTCPTimeoutAfterConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			accepted <- connection
		}
	}()

	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	serverConnection := <-accepted
	defer serverConnection.Close()
	if err := setIntegrationDeadline(connection); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	_, err = connection.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("error = %v, want timeout", err)
	}
	if elapsed := time.Since(started); elapsed < 4*time.Second || elapsed > 10*time.Second {
		t.Fatalf("TCP timeout elapsed = %v", elapsed)
	}
}

type upstreamCapture struct {
	mu       sync.Mutex
	requests [][]byte
}

func (c *upstreamCapture) record(body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, bytes.Clone(body))
}

func (c *upstreamCapture) requestCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

func (c *upstreamCapture) lastRequest() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.requests) == 0 {
		return nil
	}
	return bytes.Clone(c.requests[len(c.requests)-1])
}

type mockUpstream struct {
	*httptest.Server
	*upstreamCapture
}

type cpaInstance struct {
	cmd      *exec.Cmd
	config   string
	baseURL  string
	wsURL    string
	waitDone chan error
	logPath  string
	logFile  *os.File
	logClose sync.Once
}

func (cpa *cpaInstance) closeLogFile() {
	cpa.logClose.Do(func() {
		_ = cpa.logFile.Close()
	})
}

func newMockUpstream(t *testing.T) *mockUpstream {
	t.Helper()

	capture := new(upstreamCapture)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !acceptedUpstreamPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		capture.record(body)

		var envelope struct {
			Stream bool `json:"stream"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Date", "Tue, 01 Sep 2026 00:00:00 GMT")
		w.Header().Set("X-Censorship-Fixture", "fixed")
		if !envelope.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chatcmpl-censorship-fixture","object":"chat.completion","created":1788220800,"model":"censorship-integration-model","choices":[{"index":0,"message":{"role":"assistant","content":"fixed upstream output"},"finish_reason":"stop"}]}`)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		for _, event := range []string{
			`data: {"id":"chatcmpl-censorship-fixture","object":"chat.completion.chunk","model":"censorship-integration-model","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-censorship-fixture","object":"chat.completion.chunk","model":"censorship-integration-model","choices":[{"index":0,"delta":{"content":"fixed upstream output"},"finish_reason":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-censorship-fixture","object":"chat.completion.chunk","model":"censorship-integration-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n",
		} {
			_, _ = io.WriteString(w, event)
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &mockUpstream{Server: server, upstreamCapture: capture}
}

func acceptedUpstreamPath(path string) bool {
	for _, suffix := range []string{"/chat/completions", "/completions", "/responses"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func startCPA(t *testing.T, upstreamURL string, pluginsEnabled bool, censorshipYAML string) *cpaInstance {
	t.Helper()

	paths, err := resolveCPAPaths(os.Getenv("CPA_INTEGRATION_BIN"), os.Getenv("CENSORSHIP_PLUGIN_DIR"))
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	runDir := t.TempDir()
	configPath := filepath.Join(runDir, "config.yaml")
	baseConfig := fmt.Sprintf(`host: "127.0.0.1"
port: %d
api-keys:
  - %q
openai-compatibility:
  - name: "censorship-integration"
    base-url: %q
    api-key-entries:
      - api-key: "censorship-upstream-key"
    models:
      - name: %q
        alias: %q
plugins:
  enabled: %t
  dir: %q
  configs:
`, port, downstreamKey, upstreamURL, modelName, modelName, pluginsEnabled, paths.pluginDir)
	if err := os.WriteFile(configPath, []byte(baseConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	writePluginConfig(t, configPath, censorshipYAML)

	logPath := filepath.Join(runDir, "cpa.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(paths.binary, "--config", configPath, "--no-browser")
	cmd.Dir = runDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start CPA: %v", err)
	}

	cpa := &cpaInstance{
		cmd:      cmd,
		config:   configPath,
		baseURL:  fmt.Sprintf("http://127.0.0.1:%d", port),
		wsURL:    fmt.Sprintf("ws://127.0.0.1:%d/v1/responses", port),
		waitDone: make(chan error, 1),
		logPath:  logPath,
		logFile:  logFile,
	}
	go func() {
		waitErr := cmd.Wait()
		cpa.closeLogFile()
		cpa.waitDone <- waitErr
		close(cpa.waitDone)
	}()
	t.Cleanup(func() {
		stopCPA(t, cpa)
	})

	waitForCPA(t, cpa)
	return cpa
}

func writePluginConfig(t *testing.T, path, censorshipYAML string) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const marker = "    censorship:\n"
	if index := bytes.Index(contents, []byte(marker)); index >= 0 {
		contents = contents[:index]
	}
	var replacement strings.Builder
	replacement.Write(contents)
	replacement.WriteString(marker)
	replacement.WriteString("      enabled: true\n")
	for _, line := range strings.Split(strings.TrimSuffix(censorshipYAML, "\n"), "\n") {
		if line != "" {
			replacement.WriteString("      ")
			replacement.WriteString(line)
			replacement.WriteByte('\n')
		}
	}

	if err := os.WriteFile(path, []byte(replacement.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForCPA(t *testing.T, cpa *cpaInstance) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		request, err := http.NewRequest(http.MethodGet, cpa.baseURL+"/v1/models", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+downstreamKey)
		response, err := integrationHTTPClient.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case waitErr := <-cpa.waitDone:
			t.Fatalf("CPA exited before readiness: %v\n%s", waitErr, readCPALog(cpa.logPath))
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("CPA did not become ready\n%s", readCPALog(cpa.logPath))
}

func stopCPA(t *testing.T, cpa *cpaInstance) {
	t.Helper()

	select {
	case <-cpa.waitDone:
		return
	default:
	}
	if runtime.GOOS == "windows" {
		_ = terminateCPA(cpa.cmd.Process.Pid)
		if waitForProcess(cpa.waitDone, 2*time.Second) {
			return
		}
		_ = cpa.cmd.Process.Kill()
		if !waitForProcess(cpa.waitDone, 2*time.Second) {
			t.Errorf("CPA did not exit after kill\n%s", readCPALog(cpa.logPath))
		}
		return
	}
	_ = cpa.cmd.Process.Signal(os.Interrupt)
	if waitForProcess(cpa.waitDone, 2*time.Second) {
		return
	}
	_ = terminateCPA(cpa.cmd.Process.Pid)
	if waitForProcess(cpa.waitDone, 2*time.Second) {
		return
	}
	_ = cpa.cmd.Process.Kill()
	if !waitForProcess(cpa.waitDone, 2*time.Second) {
		t.Errorf("CPA did not exit after kill\n%s", readCPALog(cpa.logPath))
	}
}

func waitForProcess(done <-chan error, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func terminateCPA(pid int) error {
	if runtime.GOOS == "windows" {
		return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T").Run()
	}
	return exec.Command("kill", "-TERM", strconv.Itoa(pid)).Run()
}

func readCPALog(path string) string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	return string(contents)
}

func postJSON(t *testing.T, target string, body []byte) (int, http.Header, []byte) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+downstreamKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := integrationHTTPClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header.Clone(), responseBody
}

func postChat(t *testing.T, cpa *cpaInstance, text string) (int, http.Header, []byte) {
	t.Helper()
	return postJSON(t, cpa.baseURL+"/v1/chat/completions", chatBody(text, false))
}

type responseTrace struct {
	Status  int
	Headers http.Header
	Chunks  [][]byte
}

func captureHTTP11Trace(t *testing.T, target, key string, body []byte) responseTrace {
	t.Helper()

	endpoint, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Scheme != "http" {
		t.Fatalf("captureHTTP11Trace requires http URL, got %q", target)
	}
	address := endpoint.Host
	if endpoint.Port() == "" {
		address = net.JoinHostPort(endpoint.Hostname(), "80")
	}
	connection, err := net.DialTimeout("tcp", address, integrationIOTimeout)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := setIntegrationDeadline(connection); err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connection", "close")
	request.Header.Set("Accept-Encoding", "identity")
	if err := request.Write(connection); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(connection)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	statusParts := strings.SplitN(strings.TrimRight(statusLine, "\r\n"), " ", 3)
	if len(statusParts) < 2 {
		t.Fatalf("invalid HTTP status line %q", statusLine)
	}
	status, err := strconv.Atoi(statusParts[1])
	if err != nil {
		t.Fatal(err)
	}
	mimeHeader, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header(mimeHeader).Clone()
	for _, key := range []string{"Date", "X-CPA-TRACE-ID"} {
		if headers.Get(key) != "" {
			headers.Set(key, "<dynamic>")
		}
	}
	trace := responseTrace{Status: status, Headers: headers}

	if strings.EqualFold(headers.Get("Transfer-Encoding"), "chunked") {
		trace.Chunks = readHTTP11Chunks(t, reader)
		return trace
	}
	contentLength := headers.Get("Content-Length")
	if contentLength == "" {
		t.Fatal("response has neither chunked transfer encoding nor Content-Length")
	}
	length, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil || length < 0 {
		t.Fatalf("invalid Content-Length %q", contentLength)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		t.Fatal(err)
	}
	trace.Chunks = [][]byte{payload}
	return trace
}

func readHTTP11Chunks(t *testing.T, reader *bufio.Reader) [][]byte {
	t.Helper()

	var chunks [][]byte
	for {
		sizeLine, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		sizeText, _, _ := strings.Cut(strings.TrimSpace(sizeLine), ";")
		size, err := strconv.ParseInt(sizeText, 16, 64)
		if err != nil || size < 0 {
			t.Fatalf("invalid HTTP chunk size %q", sizeLine)
		}
		if size == 0 {
			if _, err := textproto.NewReader(reader).ReadMIMEHeader(); err != nil {
				t.Fatal(err)
			}
			return chunks
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(reader, payload); err != nil {
			t.Fatal(err)
		}
		ending := make([]byte, 2)
		if _, err := io.ReadFull(reader, ending); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ending, []byte("\r\n")) {
			t.Fatalf("invalid HTTP chunk ending %q", ending)
		}
		chunks = append(chunks, payload)
	}
}

func chatBody(text string, stream bool) []byte {
	body, err := json.Marshal(struct {
		Model    string              `json:"model"`
		Messages []map[string]string `json:"messages"`
		Stream   bool                `json:"stream"`
	}{
		Model:    modelName,
		Messages: []map[string]string{{"role": "user", "content": text}},
		Stream:   stream,
	})
	if err != nil {
		panic(err)
	}
	return body
}

func dialResponsesWebSocket(t *testing.T, target, key string) *websocket.Conn {
	t.Helper()

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+key)
	connection, response, err := websocket.DefaultDialer.DialContext(context.Background(), target, header)
	if response != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial Responses WebSocket: %v", err)
	}
	if err := setIntegrationDeadline(connection.UnderlyingConn()); err != nil {
		_ = connection.Close()
		t.Fatal(err)
	}
	return connection
}

func setIntegrationDeadline(connection net.Conn) error {
	return connection.SetDeadline(time.Now().Add(integrationIOTimeout))
}
