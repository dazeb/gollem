package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// StdioServerTransport serves an MCP Server over stdio-style newline-delimited JSON-RPC.
type StdioServerTransport struct {
	server *Server
	reader *bufio.Reader
	writer io.WriteCloser
}

// NewStdioServerTransport binds a reusable Server to stdio-style streams.
func NewStdioServerTransport(server *Server, r io.Reader, w io.WriteCloser) *StdioServerTransport {
	if server == nil {
		server = NewServer()
	}
	transport := &StdioServerTransport{
		server: server,
		reader: bufio.NewReader(r),
		writer: w,
	}
	server.attachWriter(func(data []byte) error {
		_, err := fmt.Fprintf(w, "%s\n", data)
		return err
	})
	return transport
}

// Run serves messages until EOF or context cancellation.
func (t *StdioServerTransport) Run(ctx context.Context) error {
	if t.server != nil {
		defer func() {
			t.server.markPeerClosed()
		}()
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		line, err := t.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("mcp: stdio server read failed: %w", err)
		}

		line = []byte(strings.TrimSpace(string(line)))
		if len(line) == 0 {
			continue
		}

		var msg jsonRPCMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			_ = t.server.respond(ctx, nil, nil, &jsonRPCError{
				Code:    jsonRPCCodeParseError,
				Message: "parse error: " + err.Error(),
			})
			continue
		}

		t.server.HandleMessage(ctx, &msg)
	}
}

// Close closes the transport writer and the attached server session.
func (t *StdioServerTransport) Close() error {
	if t.server != nil {
		_ = t.server.Close()
	}
	if t.writer != nil {
		return t.writer.Close()
	}
	return nil
}

// ServeStdio is a convenience helper that runs a Server over stdio-style streams.
func ServeStdio(ctx context.Context, server *Server, r io.Reader, w io.WriteCloser) error {
	return NewStdioServerTransport(server, r, w).Run(ctx)
}
