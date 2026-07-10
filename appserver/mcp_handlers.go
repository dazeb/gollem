package appserver

import (
	"context"
	"encoding/json"
	"errors"

	appmcp "github.com/fugue-labs/gollem/appserver/mcp"
	"github.com/fugue-labs/gollem/appserver/protocol"
)

func (s *Server) handleMCPServerStatusList(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	mcpSvc, rpcErr := s.requireMCP("mcpServerStatus/list")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params appmcp.StatusListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	return mcpSvc.ListStatuses(ctx, params), nil
}

func (s *Server) handleMCPServerResourceRead(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	mcpSvc, rpcErr := s.requireMCP("mcpServer/resource/read")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params appmcp.ResourceReadParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := mcpSvc.ReadResource(ctx, params)
	if err != nil {
		return nil, mapMCPError("mcpServer/resource/read", err)
	}
	return result, nil
}

func (s *Server) handleMCPServerToolCall(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	mcpSvc, rpcErr := s.requireMCP("mcpServer/tool/call")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params appmcp.ToolCallParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	target, err := mcpSvc.ResolveToolCall(params)
	if err != nil {
		return nil, mapMCPError("mcpServer/tool/call", err)
	}
	if s.approvals == nil {
		return nil, protocol.MethodUnavailableErrorWithReason("mcpServer/tool/call", "approval service is not configured")
	}
	if err := s.approvals.MCPToolApproval(ctx, target.ServerName, target.ToolName, target.Arguments); err != nil {
		return nil, mapMCPError("mcpServer/tool/call", err)
	}
	result, err := mcpSvc.CallResolvedTool(ctx, target)
	if err != nil {
		return nil, mapMCPError("mcpServer/tool/call", err)
	}
	return result, nil
}

func (s *Server) requireMCP(method string) (*appmcp.Service, *protocol.Error) {
	if s.mcp == nil {
		return nil, protocol.MethodUnavailableErrorWithReason(method, "MCP service is not configured")
	}
	return s.mcp, nil
}

func mapMCPError(method string, err error) *protocol.Error {
	switch {
	case errors.Is(err, appmcp.ErrServerNameRequired),
		errors.Is(err, appmcp.ErrServerAlreadyExists),
		errors.Is(err, appmcp.ErrServerNotFound),
		errors.Is(err, appmcp.ErrToolNameRequired),
		errors.Is(err, appmcp.ErrResourceURIRequired):
		return invalidParams("invalid "+method+" params", err)
	case errors.Is(err, ErrApprovalRequestDenied):
		return rpcError(protocol.CodeInvalidRequest, "operation denied by approval policy", err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return rpcError(protocol.CodeInvalidRequest, "request context ended", err)
	default:
		return rpcError(protocol.CodeInternalError, method+" failed", err)
	}
}
