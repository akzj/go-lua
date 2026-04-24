package lua_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akzj/go-lua/pkg/lua"
)

// newTestServer creates a test HTTP server that echoes back request info as JSON.
func newTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(`{"message":"hello"}`))
		case "/echo":
			// Echo request method and body.
			body := make([]byte, 0)
			if r.Body != nil {
				body, _ = readAll(r.Body)
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("X-Method", r.Method)
			w.WriteHeader(200)
			fmt.Fprintf(w, "method=%s body=%s", r.Method, string(body))
		case "/headers":
			// Echo back a specific request header.
			auth := r.Header.Get("Authorization")
			ct := r.Header.Get("Content-Type")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(200)
			fmt.Fprintf(w, "auth=%s ct=%s", auth, ct)
		case "/status":
			w.WriteHeader(404)
			w.Write([]byte("not found"))
		case "/slow":
			time.Sleep(3 * time.Second)
			w.WriteHeader(200)
			w.Write([]byte("slow response"))
		case "/multi-header":
			w.Header().Add("X-Custom", "val1")
			w.Header().Add("X-Custom", "val2")
			w.WriteHeader(200)
			w.Write([]byte("ok"))
		default:
			w.WriteHeader(200)
			w.Write([]byte("ok"))
		}
	}))
}

// readAll is a small helper since io.ReadAll isn't available in the test package.
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

func TestHTTP_Get(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.get("%s/json")
		assert(resp ~= nil, "response should not be nil")
		assert(resp.status == 200, "expected status 200, got " .. tostring(resp.status))
		assert(resp.body == '{"message":"hello"}', "unexpected body: " .. resp.body)
		assert(resp.headers["content-type"] == "application/json",
			"unexpected content-type: " .. tostring(resp.headers["content-type"]))
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Get_StatusText(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.get("%s/json")
		-- status_text should contain the status code
		assert(resp.status_text ~= nil, "status_text should not be nil")
		assert(string.find(resp.status_text, "200"), "status_text should contain 200")
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Get_Status404(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.get("%s/status")
		assert(resp.status == 404, "expected 404, got " .. tostring(resp.status))
		assert(resp.body == "not found", "unexpected body: " .. resp.body)
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Post(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.post("%s/echo", {
			body = "hello world",
		})
		assert(resp ~= nil, "response should not be nil")
		assert(resp.status == 200, "expected 200, got " .. tostring(resp.status))
		assert(string.find(resp.body, "method=POST"), "expected POST method in body")
		assert(string.find(resp.body, "body=hello world"), "expected body content")
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_PostJSON(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.post("%s/headers", {
			headers = {
				["Content-Type"] = "application/json",
				["Authorization"] = "Bearer token123",
			},
			body = '{"name":"test"}',
		})
		assert(resp ~= nil, "response should not be nil")
		assert(resp.status == 200)
		assert(string.find(resp.body, "auth=Bearer token123"),
			"expected auth header, got: " .. resp.body)
		assert(string.find(resp.body, "ct=application/json"),
			"expected content-type header, got: " .. resp.body)
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Request_PUT(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.request({
			method = "PUT",
			url = "%s/echo",
			body = "updated data",
		})
		assert(resp ~= nil, "response should not be nil")
		assert(resp.status == 200)
		assert(string.find(resp.body, "method=PUT"), "expected PUT method")
		assert(string.find(resp.body, "body=updated data"), "expected body content")
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Request_DELETE(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.request({
			method = "DELETE",
			url = "%s/echo",
		})
		assert(resp ~= nil, "response should not be nil")
		assert(resp.status == 200)
		assert(string.find(resp.body, "method=DELETE"), "expected DELETE method")
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Request_DefaultGET(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.request({
			url = "%s/echo",
		})
		assert(resp ~= nil, "response should not be nil")
		assert(string.find(resp.body, "method=GET"), "expected default GET method")
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Headers_CustomRequest(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.get("%s/headers", {
			headers = {
				["Authorization"] = "Bearer mytoken",
				["Content-Type"] = "text/xml",
			},
		})
		assert(resp ~= nil)
		assert(string.find(resp.body, "auth=Bearer mytoken"),
			"expected auth header, got: " .. resp.body)
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_ResponseHeaders_MultiValue(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.get("%s/multi-header")
		assert(resp ~= nil)
		local custom = resp.headers["x-custom"]
		assert(custom ~= nil, "expected x-custom header")
		-- Multi-value headers should be joined by ", "
		assert(string.find(custom, "val1"), "expected val1 in x-custom: " .. custom)
		assert(string.find(custom, "val2"), "expected val2 in x-custom: " .. custom)
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Error_InvalidURL(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local http = require("http")
		local resp, err = http.get("://invalid")
		assert(resp == nil, "expected nil response for invalid URL")
		assert(err ~= nil, "expected error string")
		assert(type(err) == "string", "expected error to be a string")
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Error_ConnectionRefused(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local http = require("http")
		local resp, err = http.get("http://127.0.0.1:1", {timeout = 1})
		assert(resp == nil, "expected nil response for refused connection")
		assert(err ~= nil, "expected error string")
		assert(type(err) == "string", "expected error to be a string")
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Timeout(t *testing.T) {
	// Use a dedicated server with a context-aware handler so
	// httptest.Server.Close() doesn't block waiting for the handler.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return // client disconnected
		case <-time.After(30 * time.Second):
			w.WriteHeader(200)
			w.Write([]byte("slow response"))
		}
	}))
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	start := time.Now()
	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp, err = http.get("%s/slow", {timeout = 0.5})
		assert(resp == nil, "expected nil response for timeout")
		assert(err ~= nil, "expected error string for timeout")
	`, ts.URL))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	// Should have timed out in ~500ms, well before the 30-second sleep.
	if elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

func TestHTTP_Context_Cancellation(t *testing.T) {
	// Use a dedicated server for context cancellation testing.
	// The handler sleeps long; the client context should cancel first.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return // client disconnected
		case <-time.After(30 * time.Second):
			w.WriteHeader(200)
			w.Write([]byte("slow response"))
		}
	}))
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	L.SetContext(ctx)

	start := time.Now()
	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp, err = http.get("%s/slow", {timeout = 30})
		assert(resp == nil, "expected nil response for cancelled context")
		assert(err ~= nil, "expected error string for cancelled context")
	`, ts.URL))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}
	// The Lua code should complete in ~500ms (context timeout),
	// NOT 30 seconds (server sleep). We allow up to 2s for CI slack.
	if elapsed > 2*time.Second {
		t.Fatalf("context cancellation took too long: %v (expected ~500ms)", elapsed)
	}
}

func TestHTTP_RequireModule(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local http = require("http")
		assert(http ~= nil, "http module should not be nil")
		assert(type(http.get) == "function", "http.get should be a function")
		assert(type(http.post) == "function", "http.post should be a function")
		assert(type(http.request) == "function", "http.request should be a function")
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_RequireModuleCached(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local http1 = require("http")
		local http2 = require("http")
		assert(http1 == http2, "require should return cached module")
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_LuaIntegration(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/data" {
			body, _ := readAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"received":%s}`, string(body))
			return
		}
		w.WriteHeader(404)
	}))
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")

		-- POST some data
		local resp = http.post("%s/api/data", {
			headers = {["Content-Type"] = "application/json"},
			body = '{"key":"value"}',
		})
		assert(resp.status == 201, "expected 201, got " .. tostring(resp.status))
		assert(resp.headers["content-type"] == "application/json")
		assert(string.find(resp.body, '"key":"value"'),
			"expected key:value in response body: " .. resp.body)
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Request_MissingURL(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	err := L.DoString(`
		local http = require("http")
		local ok, err = pcall(function()
			http.request({method = "GET"})
		end)
		assert(not ok, "expected error for missing URL")
		assert(string.find(err, "url"), "error should mention 'url': " .. err)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Get_NoOptions(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.get("%s/")
		assert(resp ~= nil)
		assert(resp.status == 200)
		assert(resp.body == "ok")
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHTTP_Post_EmptyOptions(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp = http.post("%s/echo", {})
		assert(resp ~= nil)
		assert(resp.status == 200)
		assert(string.find(resp.body, "method=POST"))
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}

// TestHTTP_LargeBody verifies that response body size is limited.
func TestHTTP_LargeBody(t *testing.T) {
	// Create a server that returns a body larger than the limit.
	bigBody := strings.Repeat("x", 11*1024*1024) // 11 MB
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(bigBody))
	}))
	defer ts.Close()

	L := lua.NewState()
	defer L.Close()

	err := L.DoString(fmt.Sprintf(`
		local http = require("http")
		local resp, err = http.get("%s")
		assert(resp == nil, "expected nil for oversized body")
		assert(err ~= nil, "expected error for oversized body")
		assert(string.find(err, "limit"), "error should mention limit: " .. err)
	`, ts.URL))
	if err != nil {
		t.Fatal(err)
	}
}
