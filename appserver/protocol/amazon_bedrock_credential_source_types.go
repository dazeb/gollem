package protocol

import "encoding/json"

// AmazonBedrockCredentialSource is the exact closed public credential
// ownership mode. It is standalone from Gollem's account and provider runtime.
type AmazonBedrockCredentialSource string

const (
	AmazonBedrockCredentialSourceCodexManaged AmazonBedrockCredentialSource = "codexManaged"
	AmazonBedrockCredentialSourceAWSManaged   AmazonBedrockCredentialSource = "awsManaged"
)

func (s AmazonBedrockCredentialSource) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(
		s,
		"Amazon Bedrock credential source",
		AmazonBedrockCredentialSource.valid,
	)
}

func (s *AmazonBedrockCredentialSource) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(
		data,
		s,
		"Amazon Bedrock credential source",
		AmazonBedrockCredentialSource.valid,
	)
}

func (s AmazonBedrockCredentialSource) valid() bool {
	return s == AmazonBedrockCredentialSourceCodexManaged ||
		s == AmazonBedrockCredentialSourceAWSManaged
}

var (
	_ json.Marshaler   = AmazonBedrockCredentialSource("")
	_ json.Unmarshaler = (*AmazonBedrockCredentialSource)(nil)
)
