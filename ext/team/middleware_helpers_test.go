package team

import (
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func requireRequestMiddleware(t testing.TB, mw core.AgentMiddleware) core.RequestMiddlewareFunc {
	t.Helper()
	if mw.Request == nil {
		t.Fatal("expected request middleware")
	}
	return mw.Request
}
