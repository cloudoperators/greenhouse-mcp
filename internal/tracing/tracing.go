package tracing

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Init(ctx context.Context, serviceName, serviceVersion, endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

func Middleware(log logr.Logger, serviceName string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			tracer := otel.Tracer(serviceName)

			extra := req.GetExtra()
			if extra != nil && extra.Header != nil {
				ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(extra.Header))
			}

			spanName := method
			var toolName string
			if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
				toolName = p.Name
				spanName = method + " " + toolName
			}

			ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()

			span.SetAttributes(attribute.String("mcp.method", method))
			if toolName != "" {
				span.SetAttributes(attribute.String("mcp.tool", toolName))
			}

			if extra != nil {
				authHeader := extra.Header.Get("Authorization")
				if oboUser := extra.Header.Get("X-On-Behalf-Of-User"); oboUser != "" {
					span.SetAttributes(attribute.String("enduser.obo", oboUser))
					if sub := extractJWTClaim(log, authHeader, "sub"); sub != "" {
						span.SetAttributes(attribute.String("enduser.id", sub))
					}
				} else if userID := extractJWTClaim(log, authHeader, "name"); userID != "" {
					span.SetAttributes(attribute.String("enduser.id", userID))
				}
			}

			result, err := next(ctx, method, req)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return result, err
		}
	}
}

func extractJWTClaim(log logr.Logger, authHeader, claim string) string {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	raw := authHeader[7:]
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		log.Error(err, "failed to parse JWT")
		return ""
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	v, ok := claims[claim].(string)
	if !ok {
		return ""
	}
	return v
}
