package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
)

// Client is a minimal JSON-RPC 2.0 HTTP client speaking the same wire
// protocol as znn_cli / Syrius against a go-zenon node's HTTP endpoint
// (default http://127.0.0.1:35997).
type Client struct {
	url        string
	httpClient *http.Client
	nextID     atomic.Int64
}

func NewClient(url string) *Client {
	return &Client{url: url, httpClient: &http.Client{}}
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message)
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// Call invokes method with the given positional params and unmarshals the
// result into out (which may be nil if the result is not needed).
func (c *Client) Call(method string, params []interface{}, out interface{}) error {
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      c.nextID.Add(1),
		Method:  method,
		Params:  params,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return fmt.Errorf("failed to decode rpc response: %w (body: %s)", err, string(respBody))
	}
	if rpcResp.Error != nil {
		return rpcResp.Error
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(rpcResp.Result, out)
}
