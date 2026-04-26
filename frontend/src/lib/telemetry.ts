import { SpanStatusCode, context, metrics, trace } from '@opentelemetry/api'
import type { Span } from '@opentelemetry/api'
import type { SpanProcessor } from '@opentelemetry/sdk-trace-base'
import { CompositePropagator, W3CBaggagePropagator, W3CTraceContextPropagator } from '@opentelemetry/core'
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http'
import { OTLPMetricExporter } from '@opentelemetry/exporter-metrics-otlp-http'
import { registerInstrumentations } from '@opentelemetry/instrumentation'
import { DocumentLoadInstrumentation } from '@opentelemetry/instrumentation-document-load'
import { FetchInstrumentation } from '@opentelemetry/instrumentation-fetch'
import { resourceFromAttributes } from '@opentelemetry/resources'
import { BatchSpanProcessor } from '@opentelemetry/sdk-trace-base'
import { WebTracerProvider } from '@opentelemetry/sdk-trace-web'
import { MeterProvider, PeriodicExportingMetricReader } from '@opentelemetry/sdk-metrics'
import { ATTR_SERVICE_NAME, ATTR_SERVICE_VERSION } from '@opentelemetry/semantic-conventions'
import { getSession } from './api/client'

const API_BASE = (import.meta.env.VITE_API_URL ?? 'http://localhost:8090').replace(/\/$/, '')

const resource = resourceFromAttributes({
  [ATTR_SERVICE_NAME]: 'babelsuite-frontend',
  [ATTR_SERVICE_VERSION]: import.meta.env.VITE_APP_VERSION ?? '0.0.0',
})

// ── User context store ───────────────────────────────────────────────────────
// Written once at login; read by UserContextProcessor on every new span so all
// traces carry identity without needing an active span at the moment of login.

type UserCtx = { userId: string; workspaceId: string; isAdmin: boolean } | null
let _userCtx: UserCtx = null

class UserContextProcessor implements SpanProcessor {
  onStart(span: Span): void {
    if (_userCtx) {
      span.setAttributes({
        'enduser.id': _userCtx.userId,
        'enduser.workspace_id': _userCtx.workspaceId,
        'enduser.is_admin': _userCtx.isAdmin,
      })
    }
  }
  onEnd(): void {}
  shutdown(): Promise<void> { return Promise.resolve() }
  forceFlush(): Promise<void> { return Promise.resolve() }
}

// ── Session-aware exporters ───────────────────────────────────────────────────
// Read the session token on every export so auth stays current across login/logout.

class SessionAwareTraceExporter extends OTLPTraceExporter {
  override export(
    spans: Parameters<OTLPTraceExporter['export']>[0],
    resultCallback: Parameters<OTLPTraceExporter['export']>[1],
  ) {
    const session = getSession()
    if (session?.token) {
      this.headers['Authorization'] = `Bearer ${session.token}`
    } else {
      delete this.headers['Authorization']
    }
    super.export(spans, resultCallback)
  }
}

class SessionAwareMetricExporter extends OTLPMetricExporter {
  override export(
    metrics: Parameters<OTLPMetricExporter['export']>[0],
    resultCallback: Parameters<OTLPMetricExporter['export']>[1],
  ) {
    const session = getSession()
    if (session?.token) {
      this.headers['Authorization'] = `Bearer ${session.token}`
    } else {
      delete this.headers['Authorization']
    }
    super.export(metrics, resultCallback)
  }
}

// ── Tracer provider ──────────────────────────────────────────────────────────

const tracerProvider = new WebTracerProvider({
  resource,
  spanProcessors: [
    new UserContextProcessor(),
    new BatchSpanProcessor(
      new SessionAwareTraceExporter({ url: `${API_BASE}/api/v1/telemetry/traces` }),
    ),
  ],
})

tracerProvider.register({
  propagator: new CompositePropagator({
    propagators: [new W3CTraceContextPropagator(), new W3CBaggagePropagator()],
  }),
})

// ── Meter provider ───────────────────────────────────────────────────────────

const meterProvider = new MeterProvider({
  resource,
  readers: [
    new PeriodicExportingMetricReader({
      exporter: new SessionAwareMetricExporter({ url: `${API_BASE}/api/v1/telemetry/metrics` }),
      exportIntervalMillis: 30_000,
    }),
  ],
})

metrics.setGlobalMeterProvider(meterProvider)

// ── Instrumentations ─────────────────────────────────────────────────────────

registerInstrumentations({
  tracerProvider,
  instrumentations: [
    new DocumentLoadInstrumentation(),
    new FetchInstrumentation({
      ignoreUrls: [/\/api\/v1\/telemetry/],
      propagateTraceHeaderCorsUrls: [new RegExp(API_BASE.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))],
    }),
  ],
})

// ── Built-in meters ──────────────────────────────────────────────────────────

const meter = metrics.getMeter('babelsuite-frontend')

export const navigationCounter = meter.createCounter('frontend.navigations', {
  description: 'Number of client-side route navigations',
})

export const apiErrorCounter = meter.createCounter('frontend.api_errors', {
  description: 'Number of API request errors',
})

export const authEventCounter = meter.createCounter('frontend.auth_events', {
  description: 'Number of sign-in and sign-up completions',
})

// ── Helpers ───────────────────────────────────────────────────────────────────

export function getTracer() {
  return trace.getTracer('babelsuite-frontend')
}

export async function withSpan<T>(name: string, fn: (span: Span) => Promise<T>): Promise<T> {
  return getTracer().startActiveSpan(name, async (span) => {
    try {
      return await fn(span)
    } catch (err) {
      if (err instanceof Error) {
        span.recordException(err)
        span.setStatus({ code: SpanStatusCode.ERROR, message: err.message })
      }
      throw err
    } finally {
      span.end()
    }
  })
}

export function setUserContext(userId: string, workspaceId: string, isAdmin: boolean) {
  _userCtx = { userId, workspaceId, isAdmin }
}

export function recordUnhandledError(error: Error, ctx?: string) {
  const span = getTracer().startSpan('unhandled_error')
  span.recordException(error)
  span.setStatus({ code: SpanStatusCode.ERROR, message: error.message })
  if (ctx) span.setAttribute('error.context', ctx)
  span.end()
}

export { context, trace, SpanStatusCode }
