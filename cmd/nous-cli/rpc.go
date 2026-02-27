package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RPCClient wraps HTTP POST calls to the nousd JSON-RPC endpoint.
type RPCClient struct {
	url    string
	client *http.Client
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int         `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
	ID      int             `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// NewRPCClient creates a client pointing at the given host:port.
func NewRPCClient(host string, port int) *RPCClient {
	return &RPCClient{
		url:    fmt.Sprintf("http://%s:%d/rpc", host, port),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Call invokes a JSON-RPC method and returns the raw result.
func (c *RPCClient) Call(method string, params interface{}) (json.RawMessage, error) {
	if params == nil {
		params = []interface{}{}
	}
	body, err := json.Marshal(rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Post(c.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}
	return rpcResp.Result, nil
}

// CallInto invokes a method and unmarshals the result into dst.
func (c *RPCClient) CallInto(dst interface{}, method string, params interface{}) error {
	raw, err := c.Call(method, params)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
