// Package protocol defines Gollem's app-server JSON-RPC contract.
//
// The app-server wire format intentionally follows Codex-style JSON-RPC
// messages with the jsonrpc member omitted on outbound messages. Decoders
// still accept a jsonrpc member so tests and generic JSON-RPC clients can
// send ordinary JSON-RPC 2.0 envelopes during bring-up.
package protocol
