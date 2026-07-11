package appserver

import (
	"encoding/json"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

func decodeThreadStatusID(raw json.RawMessage, method string) (string, *protocol.Error) {
	switch method {
	case "thread/archive":
		var params protocol.ThreadArchiveParams
		if rpcErr := decodeParams(raw, &params); rpcErr != nil {
			return "", rpcErr
		}
		return params.EffectiveThreadID(), nil
	case "thread/unarchive":
		var params protocol.ThreadUnarchiveParams
		if rpcErr := decodeParams(raw, &params); rpcErr != nil {
			return "", rpcErr
		}
		return params.EffectiveThreadID(), nil
	case "thread/delete":
		var params protocol.ThreadDeleteParams
		if rpcErr := decodeParams(raw, &params); rpcErr != nil {
			return "", rpcErr
		}
		return params.EffectiveThreadID(), nil
	default:
		return "", invalidParams("unsupported thread status method", nil)
	}
}

func protocolThreadStatusResponse(method string, thread *store.Thread) any {
	record := protocolThreadRecord(thread)
	switch method {
	case "thread/archive":
		return protocol.ThreadArchiveResponse{Thread: &record}
	case "thread/unarchive":
		return protocol.ThreadUnarchiveResponse{Thread: record}
	case "thread/delete":
		return protocol.ThreadDeleteResponse{Thread: &record}
	default:
		return struct{}{}
	}
}
