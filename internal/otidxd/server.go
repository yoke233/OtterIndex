package otidxd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"otterindex/internal/version"
)

type Options struct {
	Listen string
}

type Server struct {
	opts Options

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

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}

		if len(req.ID) == 0 {
			// Notification: no response.
			_ = s.dispatch(req)
			continue
		}

		resp := s.dispatch(req)
		_ = enc.Encode(resp)
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
	default:
		resp.Error = &ErrorObject{Code: -32601, Message: "method not found"}
	}

	return resp
}
