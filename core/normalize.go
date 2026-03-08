package core

import "context"

// NormalizeHistory returns a HistoryProcessor that cleans conversation history:
//   - Removes orphaned ToolReturnParts that have no matching ToolCallPart in a
//     preceding ModelResponse
//   - Clears Images from ToolReturnParts in completed turns (all turns except
//     the last ModelRequest), since images are useful in the turn they appear
//     but waste tokens in subsequent turns
//   - Removes ModelRequest messages that become empty after filtering
//
// The returned processor does not modify the original slice.
func NormalizeHistory() HistoryProcessor {
	return func(_ context.Context, messages []ModelMessage) ([]ModelMessage, error) {
		// Collect all tool call IDs from ModelResponse messages.
		callIDs := make(map[string]bool)
		for _, msg := range messages {
			resp, ok := msg.(ModelResponse)
			if !ok {
				continue
			}
			for _, part := range resp.Parts {
				if tc, ok := part.(ToolCallPart); ok && tc.ToolCallID != "" {
					callIDs[tc.ToolCallID] = true
				}
			}
		}

		// Find the index of the last ModelRequest for image preservation.
		lastReqIdx := -1
		for i := len(messages) - 1; i >= 0; i-- {
			if _, ok := messages[i].(ModelRequest); ok {
				lastReqIdx = i
				break
			}
		}

		result := make([]ModelMessage, 0, len(messages))
		for i, msg := range messages {
			req, ok := msg.(ModelRequest)
			if !ok {
				result = append(result, msg)
				continue
			}

			filtered := make([]ModelRequestPart, 0, len(req.Parts))
			for _, part := range req.Parts {
				tr, isTR := part.(ToolReturnPart)
				if isTR {
					// Drop orphaned tool returns.
					if tr.ToolCallID != "" && !callIDs[tr.ToolCallID] {
						continue
					}
					// Clear images from completed turns (not the last request).
					if i != lastReqIdx && len(tr.Images) > 0 {
						tr.Images = nil
						part = tr
					}
				}
				filtered = append(filtered, part)
			}

			// Skip empty requests.
			if len(filtered) == 0 {
				continue
			}

			result = append(result, ModelRequest{
				Parts:     filtered,
				Timestamp: req.Timestamp,
			})
		}
		return result, nil
	}
}
