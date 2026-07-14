package protocol

import "encoding/json"

// LoginAppBrand is the exact closed public brand for ChatGPT login UI. It is
// standalone from Gollem's account, credential, provider, and browser runtime.
type LoginAppBrand string

const (
	LoginAppBrandCodex   LoginAppBrand = "codex"
	LoginAppBrandChatGPT LoginAppBrand = "chatgpt"

	LoginAppBrandDefault = LoginAppBrandCodex
)

func (b LoginAppBrand) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(b, "login app brand", LoginAppBrand.valid)
}

func (b *LoginAppBrand) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, b, "login app brand", LoginAppBrand.valid)
}

func (b LoginAppBrand) valid() bool {
	return b == LoginAppBrandCodex || b == LoginAppBrandChatGPT
}

var (
	_ json.Marshaler   = LoginAppBrand("")
	_ json.Unmarshaler = (*LoginAppBrand)(nil)
)
