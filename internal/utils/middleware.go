// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type contextKey int

const httpMetaKey contextKey = iota

type httpMeta struct {
	ip    string
	ua    string
	auth  bool
	start time.Time
	rw    *statusRecorder
}

// InjectHTTPMeta wraps the MCP handler to inject HTTP request metadata into
// the context so CreateLoggingMiddleware can include it in every log line.
func InjectHTTPMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			ip = fwd
		}
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		meta := &httpMeta{
			ip:    ip,
			ua:    r.Header.Get("User-Agent"),
			auth:  r.Header.Get("Authorization") != "",
			start: time.Now(),
			rw:    rw,
		}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), httpMetaKey, meta)))
	})
}

func CreateLoggingMiddleware(logger logr.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()
			sessionID := req.GetSession().ID()

			fields := []any{"session", sessionID, "method", method}
			if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
				fields = append(fields, "tool", p.Name)
			}
			if meta, ok := ctx.Value(httpMetaKey).(*httpMeta); ok {
				fields = append(fields, "ip", meta.ip, "ua", meta.ua, "auth", meta.auth)
			}

			result, err := next(ctx, method, req)

			dur := time.Since(start)
			if meta, ok := ctx.Value(httpMetaKey).(*httpMeta); ok {
				fields = append(fields, "status", meta.rw.status, "latency", dur)
			} else {
				fields = append(fields, "latency", dur)
			}

			if err != nil {
				logger.Error(err, "request failed", fields...)
			} else {
				logger.Info("request ok", fields...)
			}
			return result, err
		}
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(s int) {
	r.status = s
	r.ResponseWriter.WriteHeader(s)
}
