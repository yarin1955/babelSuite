import { Code, ConnectError, type Interceptor } from '@connectrpc/connect'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { context, propagation } from '@opentelemetry/api'
import { AgentControlService, AgentRegistryService } from '../types/proto/agent/v1/agent_pb'
import { getSession, handleUnauthenticated } from './api/client'

const API_BASE = (import.meta.env.VITE_API_URL ?? 'http://localhost:8090').replace(/\/$/, '')

// Injects the W3C traceparent/tracestate/baggage headers from the currently
// active OTel span so backend Connect handlers continue the same trace.
const otelInterceptor: Interceptor = (next) => (req) => {
  propagation.inject(context.active(), req.header, {
    set(carrier, key, value) {
      carrier.set(key, value)
    },
  })
  return next(req)
}

const authInterceptor: Interceptor = (next) => (req) => {
  const session = getSession()
  if (session?.token) {
    req.header.set('Authorization', `Bearer ${session.token}`)
  }
  return next(req)
}

const errorInterceptor: Interceptor = (next) => async (req) => {
  try {
    return await next(req)
  } catch (err) {
    if (err instanceof ConnectError) {
      if (err.code === Code.Unauthenticated) {
        handleUnauthenticated()
      } else if (err.code !== Code.NotFound) {
        console.error(`[connect] ${req.method.name} failed: ${err.message}`)
      }
    }
    throw err
  }
}

const transport = createConnectTransport({
  baseUrl: API_BASE,
  useBinaryFormat: import.meta.env.PROD,
  interceptors: [otelInterceptor, authInterceptor, errorInterceptor],
})

export const agentControlClient = createClient(AgentControlService, transport)
export const agentRegistryClient = createClient(AgentRegistryService, transport)

export { Code, ConnectError }
