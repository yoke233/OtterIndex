package otidxd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"otterindex/internal/version"
)

type Options struct {
	Listen string
}

type Server struct {
	opts Options
	h    *Handlers

	mu        sync.Mutex
	listener  net.Listener
	closeOnce sync.Once
	closed    chan struct{}
}

func NewServer(opts Options) *Server {
	if opts.Listen == "" {
		opts.Listen = "127.0.0.1:7337"
	}
	return &Server{
		opts:   opts,
		h:      NewHandlers(),
		closed: make(chan struct{}),
	}
}

func (s *Server) Addr() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Run() error {
	if s == nil {
		return fmt.Errorf("server is nil")
	}

	ln, err := net.Listen("tcp", s.opts.Listen)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.isClosed() {
				return nil
			}
			return err
		}
		go s.handleConn(conn)
	}
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}

	s.closeOnce.Do(func() { close(s.closed) })

	s.mu.Lock()
	ln := s.listener
	s.listener = nil
	s.mu.Unlock()

	if ln == nil {
		return nil
	}
	return ln.Close()
}

func (s *Server) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	defer func() { _ = w.Flush() }()

	for {
		var req Request
		line, err := ReadOneLine(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}

		if err := json.Unmarshal(line, &req); err != nil {
			_ = WriteOneLine(w, Response{
				JSONRPC: "2.0",
				ID:      json.RawMessage("null"),
				Error:   &ErrorObject{Code: -32700, Message: "parse error"},
			})
			_ = w.Flush()
			continue
		}

		if len(req.ID) == 0 {
			// Notification: no response.
			_ = s.dispatch(req)
			continue
		}

		resp := s.dispatch(req)
		_ = WriteOneLine(w, resp)
		_ = w.Flush()
	}
}

func (s *Server) dispatch(req Request) Response {
	resp := Response{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		resp.Error = &ErrorObject{Code: -32600, Message: "invalid jsonrpc version"}
		return resp
	}

	switch req.Method {
	case "ping":
		resp.Result = "pong"
	case "version":
		resp.Result = version.String()
	case "workspace.add":
		var p WorkspaceAddParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &ErrorObject{Code: -32602, Message: "invalid params"}
				return resp
			}
		}
		wsid, err := s.h.WorkspaceAdd(p)
		if err != nil {
			resp.Error = &ErrorObject{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = wsid
	case "index.build":
		var p IndexBuildParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &ErrorObject{Code: -32602, Message: "invalid params"}
				return resp
			}
		}
		if strings.TrimSpace(p.WorkspaceID) == "" {
			resp.Error = &ErrorObject{Code: -32602, Message: "workspace_id is required"}
			return resp
		}
		v, err := s.h.IndexBuild(p)
		if err != nil {
			resp.Error = &ErrorObject{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = v
	case "query":
		var p QueryParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &ErrorObject{Code: -32602, Message: "invalid params"}
				return resp
			}
		}
		if strings.TrimSpace(p.WorkspaceID) == "" {
			resp.Error = &ErrorObject{Code: -32602, Message: "workspace_id is required"}
			return resp
		}
		if strings.TrimSpace(p.Q) == "" {
			resp.Error = &ErrorObject{Code: -32602, Message: "q is required"}
			return resp
		}
		items, err := s.h.Query(p)
		if err != nil {
			resp.Error = &ErrorObject{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = items
	case "watch.start":
		var p WatchStartParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &ErrorObject{Code: -32602, Message: "invalid params"}
				return resp
			}
		}
		if strings.TrimSpace(p.WorkspaceID) == "" {
			resp.Error = &ErrorObject{Code: -32602, Message: "workspace_id is required"}
			return resp
		}
		st, err := s.h.WatchStart(p)
		if err != nil {
			resp.Error = &ErrorObject{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = st
	case "watch.stop":
		var p WatchStopParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &ErrorObject{Code: -32602, Message: "invalid params"}
				return resp
			}
		}
		if strings.TrimSpace(p.WorkspaceID) == "" {
			resp.Error = &ErrorObject{Code: -32602, Message: "workspace_id is required"}
			return resp
		}
		st, err := s.h.WatchStop(p)
		if err != nil {
			resp.Error = &ErrorObject{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = st
	case "watch.status":
		var p WatchStatusParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &ErrorObject{Code: -32602, Message: "invalid params"}
				return resp
			}
		}
		if strings.TrimSpace(p.WorkspaceID) == "" {
			resp.Error = &ErrorObject{Code: -32602, Message: "workspace_id is required"}
			return resp
		}
		st, err := s.h.WatchStatus(p)
		if err != nil {
			resp.Error = &ErrorObject{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = st
	default:
		resp.Error = &ErrorObject{Code: -32601, Message: "method not found"}
	}

	return resp
}
