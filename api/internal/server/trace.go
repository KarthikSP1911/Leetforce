package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("leetforce/api")

// untraced paths are probes and scrapes that would drown real traces.
var untraced = map[string]bool{"/healthz": true, "/readyz": true}

// requestTrace starts a server span per request and puts it in the request
// context, so queue.Enqueue (and the runner after it) join the same trace. The span
// is named by the route template, never the raw path, and with tracing off it is a
// no-op.
func requestTrace() gin.HandlerFunc {
	return func(c *gin.Context) {
		if untraced[c.Request.URL.Path] {
			c.Next()
			return
		}
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := tracer.Start(ctx, c.Request.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		span.SetName(c.Request.Method + " " + route)
		span.SetAttributes(attribute.String("http.request.method", c.Request.Method),
			attribute.String("http.route", route), attribute.Int("http.response.status_code", status))
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	}
}
