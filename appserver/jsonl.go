package appserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/appserver/protocol"
)

const maxJSONLineBytes = 16 << 20

var errDaemonShutdownRequested = errors.New("appserver: daemon shutdown requested")

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
	var writeMu sync.Mutex
	var handlers sync.WaitGroup
	writeErr := make(chan error, 1)
	scheduler := NewRequestScheduler(server.RequestSchedulerLimit())

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

	eventSignal := server.NotificationSignal()
	requestSignal := server.RequestSignal()
	sendErr := func(err error) {
		if err == nil {
			return
		}
		select {
		case writeErr <- err:
		default:
		}
	}
	drainOutboundLocked := func() error {
		if err := writeJSONLineRequests(server.DrainRequests(), out); err != nil {
			return err
		}
		if err := writeJSONLineNotifications(server.DrainNotifications(), out); err != nil {
			return err
		}
		return nil
	}
	handleLine := func(line []byte, lease *RequestLease) {
		defer handlers.Done()
		err := lease.Run(ctx, func() error {
			response, hasResponse, err := handleJSONLine(ctx, server, line)
			writeMu.Lock()
			err = writeJSONLineResponsePayload(response, hasResponse, err, out)
			if err == nil {
				err = drainOutboundLocked()
			}
			if err == nil && server.DaemonShutdownRequested() {
				err = errDaemonShutdownRequested
			}
			writeMu.Unlock()
			return err
		})
		sendErr(err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-writeErr:
			if errors.Is(err, errDaemonShutdownRequested) {
				return nil
			}
			return err
		case <-requestSignal:
			writeMu.Lock()
			err := writeJSONLineRequests(server.DrainRequests(), out)
			writeMu.Unlock()
			if err != nil {
				return err
			}
		case <-eventSignal:
			writeMu.Lock()
			err := writeJSONLineNotifications(server.DrainNotifications(), out)
			writeMu.Unlock()
			if err != nil {
				return err
			}
		case scanned, ok := <-lines:
			if !ok {
				done := make(chan struct{})
				go func() {
					handlers.Wait()
					close(done)
				}()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case err := <-writeErr:
					if errors.Is(err, errDaemonShutdownRequested) {
						return nil
					}
					return err
				case <-done:
					writeMu.Lock()
					err := drainOutboundLocked()
					writeMu.Unlock()
					return err
				}
			}
			if scanned.err != nil {
				return fmt.Errorf("read app-server input: %w", scanned.err)
			}
			line := strings.TrimSpace(scanned.line)
			if line == "" {
				continue
			}
			classified, err := classifyJSONLine([]byte(line))
			if err != nil {
				writeMu.Lock()
				err = writeJSONLineResponse(ctx, server, []byte(line), out)
				writeMu.Unlock()
				if err != nil {
					return err
				}
				continue
			}
			if classified.scheduledClientRequest() {
				lease, rpcErr := scheduler.TryAcquire(classified.Method, classified.Params)
				if rpcErr != nil {
					writeMu.Lock()
					err = writeJSONLineOverloadedResponse([]byte(line), rpcErr, out)
					writeMu.Unlock()
					if err != nil {
						return err
					}
					continue
				}
				handlers.Add(1)
				go handleLine([]byte(line), lease)
				continue
			}
			handlers.Add(1)
			handleLine([]byte(line), nil)
		}
	}
}

type scannedJSONLine struct {
	line string
	err  error
}

type classifiedJSONLine struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

func (l classifiedJSONLine) isClientRequest() bool {
	return len(l.ID) > 0
}

func (l classifiedJSONLine) scheduledClientRequest() bool {
	return l.isClientRequest() && strings.TrimSpace(l.Method) != "" && l.Method != "initialize" && l.Method != "approval/respond"
}

func classifyJSONLine(line []byte) (classifiedJSONLine, error) {
	var classified classifiedJSONLine
	if err := json.Unmarshal(line, &classified); err != nil {
		return classifiedJSONLine{}, err
	}
	return classified, nil
}

func writeJSONLineResponse(ctx context.Context, server *Server, line []byte, out *bufio.Writer) error {
	response, hasResponse, err := handleJSONLine(ctx, server, line)
	return writeJSONLineResponsePayload(response, hasResponse, err, out)
}

func handleJSONLine(ctx context.Context, server *Server, line []byte) ([]byte, bool, error) {
	return server.HandleJSON(ctx, line)
}

func writeJSONLineResponsePayload(response []byte, hasResponse bool, err error, out *bufio.Writer) error {
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

func writeJSONLineRequests(requests []protocol.Request, out *bufio.Writer) error {
	for _, request := range requests {
		data, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("marshal app-server request: %w", err)
		}
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("write app-server request: %w", err)
		}
		if err := out.WriteByte('\n'); err != nil {
			return fmt.Errorf("write app-server request newline: %w", err)
		}
	}
	if len(requests) == 0 {
		return nil
	}
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush app-server requests: %w", err)
	}
	return nil
}

func writeJSONLineOverloadedResponse(line []byte, rpcErr *protocol.Error, out *bufio.Writer) error {
	var req protocol.Request
	if err := json.Unmarshal(line, &req); err != nil {
		return writeJSONLineResponsePayload(nil, false, err, out)
	}
	response, err := json.Marshal(errorResponse(req.ID, rpcErr))
	if err != nil {
		return fmt.Errorf("marshal app-server overload response: %w", err)
	}
	return writeJSONLineResponsePayload(response, true, nil, out)
}

func writeJSONLineNotifications(notifications []protocol.Notification, out *bufio.Writer) error {
	for _, notification := range notifications {
		data, err := json.Marshal(notification)
		if err != nil {
			return fmt.Errorf("marshal app-server notification: %w", err)
		}
		if _, err := out.Write(data); err != nil {
			return fmt.Errorf("write app-server notification: %w", err)
		}
		if err := out.WriteByte('\n'); err != nil {
			return fmt.Errorf("write app-server notification newline: %w", err)
		}
	}
	if len(notifications) == 0 {
		return nil
	}
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush app-server notifications: %w", err)
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
