package lua

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxResponseBody is the maximum response body size (10 MB).
const maxResponseBody = 10 * 1024 * 1024

// OpenHTTP opens the "http" module and pushes it onto the stack.
// It is registered globally via init(), so `require("http")` works
// automatically in any State.
//
// Lua API:
//
//	local http = require("http")
//	local resp = http.get(url [, options])
//	local resp = http.post(url, options)
//	local resp = http.request(options)
//
// Response table:
//
//	{
//	    status      = 200,           -- HTTP status code (integer)
//	    status_text = "200 OK",      -- full status text
//	    body        = "...",         -- response body as string
//	    headers     = {              -- response headers (lowercase keys)
//	        ["content-type"] = "application/json",
//	    },
//	}
//
// On error, all functions return nil, error_string.
func OpenHTTP(L *State) {
	L.NewLib(map[string]Function{
		"get":     httpGet,
		"post":    httpPost,
		"request": httpRequest,
	})
}

func init() {
	RegisterGlobal("http", OpenHTTP)
}

// httpGet performs an HTTP GET request.
// Lua: http.get(url [, options]) → response_table | nil, error_string
func httpGet(L *State) int {
	url := L.CheckString(1)

	var headers map[string]string
	timeout := 30 * time.Second

	if L.GetTop() >= 2 && L.IsTable(2) {
		headers = extractHeaders(L, 2)
		timeout = extractTimeout(L, 2, timeout)
	}

	return doHTTPRequest(L, "GET", url, "", headers, timeout)
}

// httpPost performs an HTTP POST request.
// Lua: http.post(url, options) → response_table | nil, error_string
func httpPost(L *State) int {
	url := L.CheckString(1)

	var headers map[string]string
	var body string
	timeout := 30 * time.Second

	if L.GetTop() >= 2 && L.IsTable(2) {
		headers = extractHeaders(L, 2)
		body = L.GetFieldString(2, "body")
		timeout = extractTimeout(L, 2, timeout)
	}

	return doHTTPRequest(L, "POST", url, body, headers, timeout)
}

// httpRequest performs a generic HTTP request.
// Lua: http.request(options) → response_table | nil, error_string
func httpRequest(L *State) int {
	L.CheckType(1, TypeTable)

	method := L.GetFieldString(1, "method")
	if method == "" {
		method = "GET"
	}
	url := L.GetFieldString(1, "url")
	if url == "" {
		L.ArgError(1, "options table must have a 'url' field")
		return 0 // unreachable
	}
	body := L.GetFieldString(1, "body")
	headers := extractHeaders(L, 1)
	timeout := extractTimeout(L, 1, 30*time.Second)

	return doHTTPRequest(L, method, url, body, headers, timeout)
}

// doHTTPRequest is the shared implementation for all HTTP functions.
// Returns 1 (response table) on success, or 2 (nil + error string) on failure.
func doHTTPRequest(L *State, method, url, body string, headers map[string]string, timeout time.Duration) int {
	// Build request body reader.
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Use State's context if available (enables cancellation from Go).
	if ctx := L.Context(); ctx != nil {
		req = req.WithContext(ctx)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}
	defer resp.Body.Close()

	// Read body with size limit.
	limitedBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		L.PushNil()
		L.PushString(err.Error())
		return 2
	}
	if len(limitedBody) > maxResponseBody {
		L.PushNil()
		L.PushString(fmt.Sprintf("response body exceeds %d bytes limit", maxResponseBody))
		return 2
	}

	// Build response table.
	L.NewTable()

	L.PushInteger(int64(resp.StatusCode))
	L.SetField(-2, "status")

	L.PushString(resp.Status)
	L.SetField(-2, "status_text")

	L.PushString(string(limitedBody))
	L.SetField(-2, "body")

	// Headers sub-table (lowercase keys, multi-values joined by ", ").
	L.NewTable()
	for k, v := range resp.Header {
		L.PushString(strings.Join(v, ", "))
		L.SetField(-2, strings.ToLower(k))
	}
	L.SetField(-2, "headers")

	return 1
}

// extractHeaders reads the "headers" field from the options table at idx.
// Returns nil if no headers field is present.
func extractHeaders(L *State, idx int) map[string]string {
	h := L.GetFieldAny(idx, "headers")
	if h == nil {
		return nil
	}
	hm, ok := h.(map[string]any)
	if !ok {
		return nil
	}
	headers := make(map[string]string, len(hm))
	for k, v := range hm {
		headers[k] = fmt.Sprintf("%v", v)
	}
	return headers
}

// extractTimeout reads the "timeout" field from the options table at idx.
// Returns the default if no timeout field is present or it's zero.
func extractTimeout(L *State, idx int, def time.Duration) time.Duration {
	t := L.GetFieldNumber(idx, "timeout")
	if t > 0 {
		return time.Duration(t * float64(time.Second))
	}
	return def
}
