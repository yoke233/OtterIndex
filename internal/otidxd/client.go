package otidxd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"otterindex/internal/model"
)

type RPCError struct {
	Code    int
	Message string
}

func (e *RPCError) Error() string { return fmt.Sprintf("rpc error (%d): %s", e.Code, e.Message) }

type Client struct {
	conn   net.Conn
	r      *bufio.Reader
	w      *bufio.Writer
	nextID int64
}

func Dial(addr string) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
	}, nil
}

func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

type rawResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

func (c *Client) call(method string, params any, out any) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("client is nil")
	}
	id := atomic.AddInt64(&c.nextID, 1)
	req := Request{JSONRPC: "2.0", Method: method, ID: json.RawMessage(fmt.Sprintf("%d", id))}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = b
	}

	if err := WriteOneLine(c.w, req); err != nil {
		return err
	}
	if err := c.w.Flush(); err != nil {
		return err
	}

	line, err := ReadOneLine(c.r)
	if err != nil {
		return err
	}
	var resp rawResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return &RPCError{Code: resp.Error.Code, Message: resp.Error.Message}
	}
	if out == nil || len(resp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}

func (c *Client) Ping() error {
	var out string
	if err := c.call("ping", nil, &out); err != nil {
		return err
	}
	if out != "pong" {
		return fmt.Errorf("unexpected ping result: %q", out)
	}
	return nil
}

func (c *Client) Version() (string, error) {
	var out string
	if err := c.call("version", nil, &out); err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) WorkspaceAdd(p WorkspaceAddParams) (string, error) {
	var out string
	if err := c.call("workspace.add", p, &out); err != nil {
		return "", err
	}
	return out, nil
}

func (c *Client) IndexBuild(p IndexBuildParams) (any, error) {
	var out any
	if err := c.call("index.build", p, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Query(p QueryParams) ([]model.ResultItem, error) {
	var out []model.ResultItem
	if err := c.call("query", p, &out); err != nil {
		return nil, err
	}
	return out, nil
}
