package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// ClientRequest is the exact standalone public client-to-server request union.
// It stays separate from Gollem's live request envelopes and method bindings.
type ClientRequest struct {
	Method string          `json:"method"`
	ID     RequestId       `json:"id"`
	Params json.RawMessage `json:"params"`
}

// ServerNotification is the exact standalone public server-to-client
// notification union, including the optional source envelope timestamp.
// It stays separate from Gollem's live notification dispatch and bindings.
type ServerNotification struct {
	Method      string          `json:"method"`
	Params      json.RawMessage `json:"params"`
	EmittedAtMS *int64          `json:"emittedAtMs,omitempty"`
}

type topLevelEnvelopeParam struct {
	typeName      string
	schema        Schema
	allowsNull    bool
	allowsNonNull bool
}

type topLevelEnvelopeContract struct {
	name     string
	schema   Schema
	variants map[string]topLevelEnvelopeParam
}

var (
	clientRequestEnvelopeContract      = mustTopLevelEnvelopeContract("client request", clientRequestSourceSchema)
	serverNotificationEnvelopeContract = mustTopLevelEnvelopeContract("server notification", serverNotificationSourceSchema)

	topLevelEnvelopeWireTypes struct {
		once  sync.Once
		types map[string]reflect.Type
	}
	topLevelEnvelopeSchemaDefinitions struct {
		once sync.Once
		defs Schema
	}
)

var topLevelEnvelopeIntegerPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func (r ClientRequest) MarshalJSON() ([]byte, error) {
	if r.ID.IsZero() {
		return nil, errors.New("client request requires id")
	}
	params, err := canonicalTopLevelEnvelopeParams(clientRequestEnvelopeContract, r.Method, r.Params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Method string          `json:"method"`
		ID     RequestId       `json:"id"`
		Params json.RawMessage `json:"params"`
	}{Method: r.Method, ID: r.ID, Params: params})
}

func (r *ClientRequest) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode client request into nil receiver")
	}
	const objectName = "client request"
	payload, err := decodeRustSerdeObject(data, objectName, "method", "id", "params")
	if err != nil {
		return err
	}
	method, err := decodeRequiredThreadItemValue[string](payload, objectName, "method")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[RequestId](payload, objectName, "id")
	if err != nil {
		return err
	}
	paramsRaw, ok := payload["params"]
	if !ok {
		return fmt.Errorf("%s requires params", objectName)
	}
	params, err := canonicalTopLevelEnvelopeParams(clientRequestEnvelopeContract, method, paramsRaw)
	if err != nil {
		return err
	}
	*r = ClientRequest{Method: method, ID: id, Params: params}
	return nil
}

func (n ServerNotification) MarshalJSON() ([]byte, error) {
	params, err := canonicalTopLevelEnvelopeParams(serverNotificationEnvelopeContract, n.Method, n.Params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Method      string          `json:"method"`
		Params      json.RawMessage `json:"params"`
		EmittedAtMS *int64          `json:"emittedAtMs,omitempty"`
	}{Method: n.Method, Params: params, EmittedAtMS: n.EmittedAtMS})
}

func (n *ServerNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode server notification into nil receiver")
	}
	const objectName = "server notification"
	payload, err := decodeRustSerdeObject(data, objectName, "method", "params", "emittedAtMs")
	if err != nil {
		return err
	}
	method, err := decodeRequiredThreadItemValue[string](payload, objectName, "method")
	if err != nil {
		return err
	}
	paramsRaw, ok := payload["params"]
	if !ok {
		return fmt.Errorf("%s requires params", objectName)
	}
	params, err := canonicalTopLevelEnvelopeParams(serverNotificationEnvelopeContract, method, paramsRaw)
	if err != nil {
		return err
	}
	emittedAtMS, err := decodeOptionalTopLevelEnvelopeInt64(payload, objectName, "emittedAtMs")
	if err != nil {
		return err
	}
	*n = ServerNotification{Method: method, Params: params, EmittedAtMS: emittedAtMS}
	return nil
}

func clientRequestSchema() Schema {
	return mustTopLevelEnvelopeSourceSchema(clientRequestSourceSchema)
}

func serverNotificationSchema() Schema {
	return mustTopLevelEnvelopeSourceSchema(serverNotificationSourceSchema)
}

func mustTopLevelEnvelopeSourceSchema(source string) Schema {
	schema, err := topLevelEnvelopeSourceSchema(source)
	if err != nil {
		panic(fmt.Sprintf("decode top-level envelope source schema: %v", err))
	}
	return schema
}

func topLevelEnvelopeTypeScript(contract topLevelEnvelopeContract, includeTimestamp bool) (string, error) {
	variants, ok := contract.schema["oneOf"].([]any)
	if !ok || len(variants) == 0 {
		return "", fmt.Errorf("%s schema has no TypeScript variants", contract.name)
	}
	parts := make([]string, 0, len(variants))
	for _, rawVariant := range variants {
		variant, ok := topLevelEnvelopeSchemaValue(rawVariant)
		if !ok {
			return "", fmt.Errorf("%s TypeScript variant is invalid", contract.name)
		}
		properties, ok := topLevelEnvelopeSchemaValue(variant["properties"])
		if !ok {
			return "", fmt.Errorf("%s TypeScript variant has no properties", contract.name)
		}
		methodSchema, ok := topLevelEnvelopeSchemaValue(properties["method"])
		if !ok {
			return "", fmt.Errorf("%s TypeScript variant has no method", contract.name)
		}
		methods, ok := methodSchema["enum"].([]any)
		if !ok || len(methods) != 1 {
			return "", fmt.Errorf("%s TypeScript variant has invalid method", contract.name)
		}
		method, ok := methods[0].(string)
		if !ok {
			return "", fmt.Errorf("%s TypeScript method is not a string", contract.name)
		}
		paramsType, err := typeScriptType(properties["params"], 0)
		if err != nil {
			return "", fmt.Errorf("%s TypeScript params for %s: %w", contract.name, method, err)
		}
		fields := []string{`"method": ` + typeScriptString(method), `"params": ` + paramsType}
		if contract.name == "client request" {
			idType, err := typeScriptType(properties["id"], 0)
			if err != nil {
				return "", fmt.Errorf("%s TypeScript id for %s: %w", contract.name, method, err)
			}
			fields = []string{`"method": ` + typeScriptString(method), `"id": ` + idType, `"params": ` + paramsType}
		}
		parts = append(parts, `{ `+strings.Join(fields, `; `)+`; }`)
	}
	union := strings.Join(parts, " | ")
	if !includeTimestamp {
		return union, nil
	}
	properties, ok := topLevelEnvelopeSchemaValue(contract.schema["properties"])
	if !ok {
		return "", fmt.Errorf("%s schema has no timestamp properties", contract.name)
	}
	timestampType, err := typeScriptType(properties["emittedAtMs"], 0)
	if err != nil {
		return "", fmt.Errorf("%s TypeScript timestamp: %w", contract.name, err)
	}
	return `{ "emittedAtMs"?: ` + timestampType + `; } & (` + union + `)`, nil
}

func canonicalTopLevelEnvelopeParams(
	contract topLevelEnvelopeContract,
	method string,
	raw json.RawMessage,
) (json.RawMessage, error) {
	variant, ok := contract.variants[method]
	if !ok {
		return nil, fmt.Errorf("unknown %s method %q", contract.name, method)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%s params for %s must contain JSON", contract.name, method)
	}
	if err := validateTopLevelEnvelopeSchema(trimmed, variant.schema, topLevelEnvelopeDefinitions()); err != nil {
		return nil, fmt.Errorf("decode %s params for %s: %w", contract.name, method, err)
	}
	if isJSONNull(trimmed) {
		if !variant.allowsNull {
			return nil, fmt.Errorf("%s params for %s must not be null", contract.name, method)
		}
		return json.RawMessage("null"), nil
	}
	if !variant.allowsNonNull {
		return nil, fmt.Errorf("%s params for %s must be null", contract.name, method)
	}
	return canonicalTopLevelEnvelopeWireValue(contract.name, method, variant.typeName, trimmed)
}

func canonicalTopLevelEnvelopeWireValue(
	contractName, method, typeName string, raw json.RawMessage,
) (json.RawMessage, error) {
	typeForName, ok := topLevelEnvelopeWireType(typeName)
	if !ok {
		return nil, fmt.Errorf("%s params for %s reference unregistered type %q", contractName, method, typeName)
	}
	value := reflect.New(typeForName)
	if err := json.Unmarshal(raw, value.Interface()); err != nil {
		return nil, fmt.Errorf("decode %s params for %s: %w", contractName, method, err)
	}
	canonical, err := json.Marshal(value.Elem().Interface())
	if err != nil {
		return nil, fmt.Errorf("encode %s params for %s: %w", contractName, method, err)
	}
	return canonical, nil
}

func topLevelEnvelopeWireType(name string) (reflect.Type, bool) {
	topLevelEnvelopeWireTypes.once.Do(func() {
		topLevelEnvelopeWireTypes.types = make(map[string]reflect.Type, len(wireSchemaDefinitionTypes()))
		for _, definition := range wireSchemaDefinitionTypes() {
			topLevelEnvelopeWireTypes.types[definition.Name] = definition.Type
		}
	})
	typeForName, ok := topLevelEnvelopeWireTypes.types[name]
	return typeForName, ok
}

func topLevelEnvelopeDefinitions() Schema {
	topLevelEnvelopeSchemaDefinitions.once.Do(func() {
		topLevelEnvelopeSchemaDefinitions.defs = JSONSchema()["$defs"].(Schema)
	})
	return topLevelEnvelopeSchemaDefinitions.defs
}

func decodeOptionalTopLevelEnvelopeInt64(
	payload map[string]json.RawMessage,
	objectName, fieldName string,
) (*int64, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func mustTopLevelEnvelopeContract(name, source string) topLevelEnvelopeContract {
	schema, err := topLevelEnvelopeSourceSchema(source)
	if err != nil {
		panic(fmt.Sprintf("decode %s source schema: %v", name, err))
	}
	variants, ok := schema["oneOf"].([]any)
	if !ok || len(variants) == 0 {
		panic(fmt.Sprintf("decode %s source schema: missing oneOf variants", name))
	}
	contract := topLevelEnvelopeContract{name: name, schema: schema, variants: make(map[string]topLevelEnvelopeParam, len(variants))}
	for _, rawVariant := range variants {
		variant, ok := topLevelEnvelopeSchemaValue(rawVariant)
		if !ok {
			panic(fmt.Sprintf("decode %s source schema: invalid variant", name))
		}
		properties, ok := topLevelEnvelopeSchemaValue(variant["properties"])
		if !ok {
			panic(fmt.Sprintf("decode %s source schema: variant properties missing", name))
		}
		methodSchema, ok := topLevelEnvelopeSchemaValue(properties["method"])
		if !ok {
			panic(fmt.Sprintf("decode %s source schema: variant method missing", name))
		}
		methodValues, ok := methodSchema["enum"].([]any)
		if !ok || len(methodValues) != 1 {
			panic(fmt.Sprintf("decode %s source schema: invalid method enum", name))
		}
		method, ok := methodValues[0].(string)
		if !ok || method == "" {
			panic(fmt.Sprintf("decode %s source schema: invalid method", name))
		}
		paramsSchema, ok := topLevelEnvelopeSchemaValue(properties["params"])
		if !ok {
			panic(fmt.Sprintf("decode %s source schema: params missing for %s", name, method))
		}
		params, err := topLevelEnvelopeParamSchema(paramsSchema)
		if err != nil {
			panic(fmt.Sprintf("decode %s source schema for %s: %v", name, method, err))
		}
		if _, exists := contract.variants[method]; exists {
			panic(fmt.Sprintf("decode %s source schema: duplicate method %s", name, method))
		}
		contract.variants[method] = params
	}
	return contract
}

func topLevelEnvelopeSourceSchema(source string) (Schema, error) {
	var value any
	if err := json.Unmarshal([]byte(source), &value); err != nil {
		return nil, err
	}
	schema, ok := topLevelEnvelopeSchemaValue(topLevelEnvelopeTranslateRefs(value))
	if !ok {
		return nil, errors.New("source schema must be an object")
	}
	return schema, nil
}

func topLevelEnvelopeTranslateRefs(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		translated := make(Schema, len(typed))
		for name, nested := range typed {
			translated[name] = topLevelEnvelopeTranslateRefs(nested)
		}
		return translated
	case []any:
		translated := make([]any, len(typed))
		for index, nested := range typed {
			translated[index] = topLevelEnvelopeTranslateRefs(nested)
		}
		return translated
	case string:
		return strings.ReplaceAll(typed, "#/definitions/", "#/$defs/")
	default:
		return value
	}
}

func topLevelEnvelopeSchemaValue(value any) (Schema, bool) {
	switch typed := value.(type) {
	case Schema:
		return typed, true
	case map[string]any:
		return Schema(typed), true
	default:
		return nil, false
	}
}

func topLevelEnvelopeParamSchema(schema Schema) (topLevelEnvelopeParam, error) {
	if reference, ok := schema["$ref"].(string); ok {
		const prefix = "#/$defs/"
		if !strings.HasPrefix(reference, prefix) || len(reference) == len(prefix) {
			return topLevelEnvelopeParam{}, fmt.Errorf("unsupported params ref %q", reference)
		}
		return topLevelEnvelopeParam{typeName: strings.TrimPrefix(reference, prefix), schema: schema, allowsNonNull: true}, nil
	}
	if schema["type"] == "null" {
		return topLevelEnvelopeParam{schema: schema, allowsNull: true}, nil
	}
	variants, ok := schema["anyOf"].([]any)
	if !ok || len(variants) == 0 {
		return topLevelEnvelopeParam{}, errors.New("params must be a ref, null, or anyOf")
	}
	var result topLevelEnvelopeParam
	for _, rawVariant := range variants {
		variant, ok := topLevelEnvelopeSchemaValue(rawVariant)
		if !ok {
			return topLevelEnvelopeParam{}, errors.New("params anyOf contains a non-schema")
		}
		parsed, err := topLevelEnvelopeParamSchema(variant)
		if err != nil {
			return topLevelEnvelopeParam{}, err
		}
		if parsed.allowsNonNull {
			if result.allowsNonNull && result.typeName != parsed.typeName {
				return topLevelEnvelopeParam{}, errors.New("params anyOf has multiple non-null types")
			}
			result.typeName = parsed.typeName
			result.allowsNonNull = true
		}
		result.allowsNull = result.allowsNull || parsed.allowsNull
	}
	result.schema = schema
	return result, nil
}

func validateTopLevelEnvelopeSchema(raw json.RawMessage, schema Schema, definitions Schema) error {
	if reference, ok := schema["$ref"].(string); ok {
		const prefix = "#/$defs/"
		if !strings.HasPrefix(reference, prefix) || len(reference) == len(prefix) {
			return fmt.Errorf("unsupported schema ref %q", reference)
		}
		name := strings.TrimPrefix(reference, prefix)
		target, ok := definitions[name]
		if !ok {
			return fmt.Errorf("schema ref %q is not registered", reference)
		}
		targetSchema, ok := topLevelEnvelopeSchemaValue(target)
		if !ok {
			return fmt.Errorf("schema ref %q is not an object", reference)
		}
		return validateTopLevelEnvelopeSchema(raw, targetSchema, definitions)
	}
	if allOf, ok := schema["allOf"].([]any); ok {
		for _, rawVariant := range allOf {
			variant, ok := topLevelEnvelopeSchemaValue(rawVariant)
			if !ok {
				return errors.New("schema allOf contains a non-schema")
			}
			if err := validateTopLevelEnvelopeSchema(raw, variant, definitions); err != nil {
				return err
			}
		}
	}
	if oneOf, ok := schema["oneOf"].([]any); ok {
		matches := 0
		for _, rawVariant := range oneOf {
			variant, ok := topLevelEnvelopeSchemaValue(rawVariant)
			if !ok {
				return errors.New("schema oneOf contains a non-schema")
			}
			if validateTopLevelEnvelopeSchema(raw, variant, definitions) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("value matches %d oneOf variants", matches)
		}
	}
	if anyOf, ok := schema["anyOf"].([]any); ok {
		matched := false
		for _, rawVariant := range anyOf {
			variant, ok := topLevelEnvelopeSchemaValue(rawVariant)
			if !ok {
				return errors.New("schema anyOf contains a non-schema")
			}
			if validateTopLevelEnvelopeSchema(raw, variant, definitions) == nil {
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("value does not match anyOf variants")
		}
	}
	if values, ok := schema["enum"].([]any); ok {
		if err := validateTopLevelEnvelopeEnum(raw, values); err != nil {
			return err
		}
	}
	if types, ok := schema["type"].([]any); ok {
		for _, rawType := range types {
			typeName, ok := rawType.(string)
			if ok && validateTopLevelEnvelopeType(raw, typeName, schema, definitions) == nil {
				return nil
			}
		}
		return fmt.Errorf("value does not match schema types %v", types)
	}
	if typeName, ok := schema["type"].(string); ok {
		return validateTopLevelEnvelopeType(raw, typeName, schema, definitions)
	}
	if _, ok := schema["properties"]; ok {
		return validateTopLevelEnvelopeObject(raw, schema, definitions)
	}
	_, err := topLevelEnvelopeJSONValue(raw)
	return err
}

func validateTopLevelEnvelopeType(raw json.RawMessage, typeName string, schema Schema, definitions Schema) error {
	value, err := topLevelEnvelopeJSONValue(raw)
	if err != nil {
		return err
	}
	switch typeName {
	case "null":
		if value != nil {
			return errors.New("value must be null")
		}
		return nil
	case "boolean":
		if _, ok := value.(bool); !ok {
			return errors.New("value must be a boolean")
		}
		return nil
	case "string":
		text, ok := value.(string)
		if !ok {
			return errors.New("value must be a string")
		}
		return validateTopLevelEnvelopeString(text, schema)
	case "integer":
		number, ok := value.(json.Number)
		if !ok || !topLevelEnvelopeIntegerPattern.MatchString(number.String()) {
			return errors.New("value must be an integer")
		}
		return validateTopLevelEnvelopeNumber(number, schema)
	case "number":
		number, ok := value.(json.Number)
		if !ok {
			return errors.New("value must be a number")
		}
		return validateTopLevelEnvelopeNumber(number, schema)
	case "array":
		if _, ok := value.([]any); !ok {
			return errors.New("value must be an array")
		}
		return validateTopLevelEnvelopeArray(raw, schema, definitions)
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return errors.New("value must be an object")
		}
		return validateTopLevelEnvelopeObject(raw, schema, definitions)
	default:
		return fmt.Errorf("unsupported schema type %q", typeName)
	}
}

func validateTopLevelEnvelopeString(text string, schema Schema) error {
	if minimum, ok := topLevelEnvelopeInt(schema["minLength"]); ok && utf8.RuneCountInString(text) < minimum {
		return fmt.Errorf("string length must be at least %d", minimum)
	}
	if maximum, ok := topLevelEnvelopeInt(schema["maxLength"]); ok && utf8.RuneCountInString(text) > maximum {
		return fmt.Errorf("string length must be at most %d", maximum)
	}
	if pattern, ok := schema["pattern"].(string); ok {
		expression, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("compile schema pattern %q: %w", pattern, err)
		}
		if !expression.MatchString(text) {
			return fmt.Errorf("string does not match pattern %q", pattern)
		}
	}
	return nil
}

func validateTopLevelEnvelopeNumber(number json.Number, schema Schema) error {
	value, ok := new(big.Rat).SetString(number.String())
	if !ok {
		return fmt.Errorf("invalid JSON number %q", number.String())
	}
	for _, bound := range []struct {
		name   string
		value  any
		accept func(int) bool
	}{
		{"minimum", schema["minimum"], func(compare int) bool { return compare >= 0 }},
		{"maximum", schema["maximum"], func(compare int) bool { return compare <= 0 }},
	} {
		if bound.value == nil {
			continue
		}
		encoded, err := json.Marshal(bound.value)
		if err != nil {
			return fmt.Errorf("encode schema %s: %w", bound.name, err)
		}
		limit, ok := new(big.Rat).SetString(string(encoded))
		if !ok {
			return fmt.Errorf("invalid schema %s %s", bound.name, encoded)
		}
		if !bound.accept(value.Cmp(limit)) {
			return fmt.Errorf("number violates %s", bound.name)
		}
	}
	return nil
}

func validateTopLevelEnvelopeArray(raw json.RawMessage, schema Schema, definitions Schema) error {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	if minimum, ok := topLevelEnvelopeInt(schema["minItems"]); ok && len(values) < minimum {
		return fmt.Errorf("array length must be at least %d", minimum)
	}
	if maximum, ok := topLevelEnvelopeInt(schema["maxItems"]); ok && len(values) > maximum {
		return fmt.Errorf("array length must be at most %d", maximum)
	}
	items, ok := topLevelEnvelopeSchemaValue(schema["items"])
	if !ok {
		return nil
	}
	for _, value := range values {
		if err := validateTopLevelEnvelopeSchema(value, items, definitions); err != nil {
			return err
		}
	}
	return nil
}

func validateTopLevelEnvelopeObject(raw json.RawMessage, schema Schema, definitions Schema) error {
	properties, _ := topLevelEnvelopeSchemaValue(schema["properties"])
	fields, err := topLevelEnvelopeObjectFields(raw, properties)
	if err != nil {
		return err
	}
	required := topLevelEnvelopeRequiredNames(schema["required"])
	for name := range required {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("object requires %s", name)
		}
	}
	additional, hasAdditionalSchema := topLevelEnvelopeSchemaValue(schema["additionalProperties"])
	for name, value := range fields {
		property, known := properties[name]
		if known {
			propertySchema, _ := topLevelEnvelopeSchemaValue(property)
			if err := validateTopLevelEnvelopeSchema(value, propertySchema, definitions); err != nil {
				return fmt.Errorf("property %s: %w", name, err)
			}
			continue
		}
		if hasAdditionalSchema {
			if err := validateTopLevelEnvelopeSchema(value, additional, definitions); err != nil {
				return fmt.Errorf("additional property %s: %w", name, err)
			}
		}
	}
	return nil
}

func validateTopLevelEnvelopeEnum(raw json.RawMessage, values []any) error {
	var candidate any
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return err
	}
	for _, value := range values {
		if reflect.DeepEqual(candidate, value) {
			return nil
		}
	}
	return fmt.Errorf("value is not one of %v", values)
}

func topLevelEnvelopeJSONValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("JSON value has trailing data")
		}
		return nil, err
	}
	return value, nil
}

func topLevelEnvelopeObjectFields(raw json.RawMessage, properties Schema) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if opening != json.Delim('{') {
		return nil, errors.New("value must be an object")
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("object property name must be a string")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if _, known := properties[name]; known {
			if _, duplicate := fields[name]; duplicate {
				return nil, fmt.Errorf("duplicate object property %q", name)
			}
		}
		fields[name] = append(json.RawMessage(nil), value...)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("object has trailing data")
		}
		return nil, err
	}
	return fields, nil
}

func topLevelEnvelopeRequiredNames(value any) map[string]struct{} {
	result := map[string]struct{}{}
	switch typed := value.(type) {
	case []string:
		for _, name := range typed {
			result[name] = struct{}{}
		}
	case []any:
		for _, value := range typed {
			if name, ok := value.(string); ok {
				result[name] = struct{}{}
			}
		}
	}
	return result
}

func topLevelEnvelopeInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), float64(int(typed)) == typed
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

var (
	_ json.Marshaler   = ClientRequest{}
	_ json.Unmarshaler = (*ClientRequest)(nil)
	_ json.Marshaler   = ServerNotification{}
	_ json.Unmarshaler = (*ServerNotification)(nil)
)
