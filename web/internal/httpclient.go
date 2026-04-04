package internal

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"time"

	webapi "github.com/akzj/go-lua/web/api"
)

// HTTPClient provides non-blocking HTTP requests.
type HTTPClient struct {
	client  *http.Client
	timeout time.Duration
}

// HTTPResponse is the response from HTTPClient.
type HTTPResponse = webapi.HTTPResponse

// NewHTTPClient creates a new HTTPClient.
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		client: &http.Client{Timeout: 30 * time.Second},
		timeout: 30 * time.Second,
	}
}

func (c *HTTPClient) Get(url string, headers map[string]string) (*HTTPResponse, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.doRequest(req)
}

func (c *HTTPClient) Post(url string, headers map[string]string, body []byte) (*HTTPResponse, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.doRequest(req)
}

func (c *HTTPClient) Put(url string, headers map[string]string, body []byte) (*HTTPResponse, error) {
	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.doRequest(req)
}

func (c *HTTPClient) Delete(url string, headers map[string]string) (*HTTPResponse, error) {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.doRequest(req)
}

func (c *HTTPClient) doRequest(req *http.Request) (*HTTPResponse, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	header := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			header[k] = v[0]
		}
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Header:     header,
		Body:       body,
	}, nil
}
