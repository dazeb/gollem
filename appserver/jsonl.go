package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fugue-labs/gollem/appserver/protocol"
)

const maxJSONLineBytes = 16 << 20

// ServeJSONLines serves newline-delimited JSON-RPC messages over the supplied
// reader and writer. It is intended for stdio first, but tests and future
// transports can reuse it for any ordered byte stream.
func ServeJSONLines(ctx context.Context, server *Server, reader io.Reader, writer io.Writer) error {
	if server == nil {
		return errors.New("appserver: nil server")
	}
	if reader == nil {
		return errors.New("appserver: nil reader")
	}
	if writer == nil {
		return errors.New("appserver: nil writer")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), maxJSONLineBytes)
	out := bufio.NewWriter(writer)
	defer func() { _ = out.Flush() }()

	lines := make(chan scannedJSONLine, 1)
	go func() {
		defer close(lines)
		for scanner.Scan() {
			lines <- scannedJSONLine{line: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			lines <- scannedJSONLine{err: err}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case scanned, ok := <-lines:
			if !ok {
				return nil
			}
			if scanned.err != nil {
				return fmt.Errorf("read app-server input: %w", scanned.err)
			}
			line := strings.TrimSpace(scanned.line)
			if line == "" {
				continue
			}
			if err := writeJSONLineResponse(ctx, server, []byte(line), out); err != nil {
				return err
			}
		}
	}
}

type scannedJSONLine struct {
	line string
	err  error
}

func writeJSONLineResponse(ctx context.Context, server *Server, line []byte, out *bufio.Writer) error {
	response, hasResponse, err := server.HandleJSON(ctx, line)
	if err != nil {
		response = marshalTransportError(err)
		hasResponse = true
	}
	if !hasResponse {
		return nil
	}
	if _, err := out.Write(response); err != nil {
		return fmt.Errorf("write app-server response: %w", err)
	}
	if err := out.WriteByte('\n'); err != nil {
		return fmt.Errorf("write app-server response newline: %w", err)
	}
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush app-server response: %w", err)
	}
	return nil
}

func marshalTransportError(err error) []byte {
	type response struct {
		ID    any             `json:"id"`
		Error *protocol.Error `json:"error"`
	}
	rpcErr := &protocol.Error{Code: protocol.CodeInvalidRequest, Message: "invalid app-server message"}
	var protocolErr *protocol.Error
	if errors.As(err, &protocolErr) {
		rpcErr = protocolErr
	} else if err != nil {
		data, _ := json.Marshal(map[string]string{"reason": err.Error()})
		rpcErr.Data = data
	}
	data, marshalErr := json.Marshal(response{ID: nil, Error: rpcErr})
	if marshalErr != nil {
		return []byte(`{"id":null,"error":{"code":-32603,"message":"marshal transport error"}}`)
	}
	return data
}
