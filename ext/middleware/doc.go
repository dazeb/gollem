// Package middleware provides a middleware chain for intercepting model
// requests, enabling cross-cutting concerns like logging, metrics, and
// tracing without modifying provider or agent code.
//
// # Usage
//
//	metrics := &middleware.Metrics{}
//	model := middleware.Wrap(anthropic.New(),
//	    middleware.NewLogging(slog.Default(), slog.LevelInfo),
//	    middleware.NewMetrics(metrics),
//	)
//
// Middlewares are applied in order: the first middleware is outermost
// and executes first.
package middleware
