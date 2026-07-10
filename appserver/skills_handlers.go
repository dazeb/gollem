package appserver

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/fugue-labs/gollem/appserver/protocol"
	appskills "github.com/fugue-labs/gollem/appserver/skills"
)

func (s *Server) handleSkillsList(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	skillsSvc, rpcErr := s.requireSkills("skills/list")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params appskills.ListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := skillsSvc.ListSkills(ctx, params)
	if err != nil {
		return nil, mapSkillsError("skills/list", err)
	}
	return result, nil
}

func (s *Server) handlePluginList(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	skillsSvc, rpcErr := s.requireSkills("plugin/list")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params appskills.PluginListParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := skillsSvc.ListPlugins(ctx, params)
	if err != nil {
		return nil, mapSkillsError("plugin/list", err)
	}
	return result, nil
}

func (s *Server) handlePluginRead(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	skillsSvc, rpcErr := s.requireSkills("plugin/read")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params appskills.PluginReadParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := skillsSvc.ReadPlugin(ctx, params)
	if err != nil {
		return nil, mapSkillsError("plugin/read", err)
	}
	return result, nil
}

func (s *Server) handlePluginSkillRead(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	skillsSvc, rpcErr := s.requireSkills("plugin/skill/read")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params appskills.PluginSkillReadParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := skillsSvc.ReadPluginSkill(ctx, params)
	if err != nil {
		return nil, mapSkillsError("plugin/skill/read", err)
	}
	return result, nil
}

func (s *Server) requireSkills(method string) (*appskills.Service, *protocol.Error) {
	if s.skills == nil {
		return nil, protocol.MethodUnavailableErrorWithReason(method, "skills service is not configured")
	}
	return s.skills, nil
}

func mapSkillsError(method string, err error) *protocol.Error {
	switch {
	case errors.Is(err, appskills.ErrPluginNotFound),
		errors.Is(err, appskills.ErrSkillNotFound),
		errors.Is(err, appskills.ErrPathOutsideRoot):
		return invalidParams("invalid "+method+" params", err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return rpcError(protocol.CodeInvalidRequest, "request context ended", err)
	default:
		return rpcError(protocol.CodeInternalError, method+" failed", err)
	}
}
