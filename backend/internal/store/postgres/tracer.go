package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const dbScope = "github.com/babelsuite/babelsuite/internal/store/postgres"

type spanKey struct{}

type pgxTracer struct {
	tracer trace.Tracer
}

func newPGXTracer() *pgxTracer {
	return &pgxTracer{tracer: otel.Tracer(dbScope)}
}

func (t *pgxTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	spanCtx, span := t.tracer.Start(ctx, sqlSpanName(data.SQL),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemPostgreSQL,
			semconv.DBQueryTextKey.String(truncateSQL(data.SQL)),
			semconv.DBOperationNameKey.String(sqlOperation(data.SQL)),
		),
	)
	return context.WithValue(spanCtx, spanKey{}, span)
}

func (t *pgxTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(spanKey{}).(trace.Span)
	if !ok {
		return
	}
	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, data.Err.Error())
	}
	span.End()
}

func sqlSpanName(sql string) string {
	op := sqlOperation(sql)
	if op == "" {
		return "db.query"
	}
	return "db." + strings.ToLower(op)
}

func sqlOperation(sql string) string {
	s := strings.TrimSpace(sql)
	if idx := strings.IndexAny(s, " \t\n\r"); idx > 0 {
		s = s[:idx]
	}
	return strings.ToUpper(s)
}

func truncateSQL(sql string) string {
	const maxLen = 512
	if len(sql) <= maxLen {
		return sql
	}
	return sql[:maxLen] + "…"
}
