import {
  buildAuthenticatedStreamUrl,
} from '../api/client'
import type {
  ExecutionLogStreamRecord,
  ExecutionStreamEvent,
  SandboxStreamEvent,
} from '../api'
import {
  openReplayableEventStream,
  type StreamConnectionState,
} from './sse'

interface ReplayableStreamHandlers<T> {
  since?: number
  onEvent: (event: T) => void
  onStateChange?: (state: StreamConnectionState) => void
}

export function openSandboxEventStream(handlers: ReplayableStreamHandlers<SandboxStreamEvent>) {
  return openReplayableEventStream({
    initialCursor: handlers.since ?? 0,
    createUrl: (cursor) => buildAuthenticatedStreamUrl('/api/v1/sandboxes/events', {
      since: cursor > 0 ? cursor : undefined,
    }),
    onEvent: handlers.onEvent,
    onStateChange: handlers.onStateChange,
    getEventId: (event) => event.id,
  })
}

export function openExecutionEventStream(
  executionId: string,
  handlers: ReplayableStreamHandlers<ExecutionStreamEvent>,
) {
  return openReplayableEventStream({
    initialCursor: handlers.since ?? 0,
    createUrl: (cursor) => buildAuthenticatedStreamUrl(`/api/v1/executions/${encodeURIComponent(executionId)}/events`, {
      since: cursor > 0 ? cursor : undefined,
    }),
    onEvent: handlers.onEvent,
    onStateChange: handlers.onStateChange,
    getEventId: (event) => event.id,
  })
}

export function openExecutionLogStream(
  executionId: string,
  handlers: ReplayableStreamHandlers<ExecutionLogStreamRecord>,
) {
  return openReplayableEventStream({
    initialCursor: handlers.since ?? 0,
    createUrl: (cursor) => buildAuthenticatedStreamUrl(`/api/v1/executions/${encodeURIComponent(executionId)}/logs`, {
      since: cursor > 0 ? cursor : undefined,
    }),
    onEvent: handlers.onEvent,
    onStateChange: handlers.onStateChange,
    getEventId: (event) => event.id,
  })
}
