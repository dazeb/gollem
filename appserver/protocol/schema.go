package protocol

// Schema is a JSON Schema document fragment.
type Schema map[string]any

// JSONSchema returns a compact schema document for the foundational
// app-server envelope and method inventory. Concrete params/results are added
// as later packages implement each surface.
func JSONSchema() Schema {
	return Schema{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title":   "Gollem App Server Protocol",
		"type":    "object",
		"$defs": Schema{
			"RequestID": Schema{
				"oneOf": []any{
					Schema{"type": "string"},
					Schema{"type": "integer"},
				},
			},
			"Error": Schema{
				"type": "object",
				"properties": Schema{
					"code":    Schema{"type": "integer"},
					"message": Schema{"type": "string"},
					"data":    Schema{},
				},
				"required": []any{"code", "message"},
			},
			"Request": Schema{
				"type": "object",
				"properties": Schema{
					"id":     Schema{"$ref": "#/$defs/RequestID"},
					"method": methodEnumSchema(SurfaceClientRequest, SurfaceGollemExtension),
					"params": Schema{},
				},
				"required":             []any{"id", "method"},
				"additionalProperties": false,
			},
			"Notification": Schema{
				"type": "object",
				"properties": Schema{
					"method": methodEnumSchema(SurfaceServerNotification, SurfaceClientNotification),
					"params": Schema{},
				},
				"required":             []any{"method"},
				"additionalProperties": false,
			},
			"Response": Schema{
				"type": "object",
				"properties": Schema{
					"id":     Schema{"$ref": "#/$defs/RequestID"},
					"result": Schema{},
					"error":  Schema{"$ref": "#/$defs/Error"},
				},
				"required":             []any{"id"},
				"additionalProperties": false,
			},
		},
		"x-gollem-protocol-version": ProtocolVersion,
		"x-gollem-methods":          Methods(),
	}
}

func methodEnumSchema(surfaces ...Surface) Schema {
	allowed := make(map[Surface]bool, len(surfaces))
	for _, surface := range surfaces {
		allowed[surface] = true
	}
	var enum []any
	for _, info := range methodRegistry {
		if allowed[info.Surface] {
			enum = append(enum, info.Method)
		}
	}
	return Schema{"type": "string", "enum": enum}
}
