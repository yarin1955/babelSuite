export type StreamConnectionState = 'connecting' | 'live' | 'reconnecting' | 'closed'

interface ReplayableEventStreamOptions<T> {
  initialCursor?: number
  createUrl: (cursor: number) => URL
  onEvent: (event: T) => void
  onStateChange?: (state: StreamConnectionState) => void
  getEventId?: (event: T) => number | undefined
  reconnectDelayMs?: number
  parse?: (data: string) => T
}

export function openReplayableEventStream<T>(options: ReplayableEventStreamOptions<T>) {
  let source: EventSource | null = null
  let reconnectTimer: number | null = null
  let closed = false
  let cursor = options.initialCursor ?? 0

  const notify = (state: StreamConnectionState) => {
    options.onStateChange?.(state)
  }

  const clearReconnect = () => {
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  const connect = (reconnecting: boolean) => {
    if (closed) {
      return
    }

    notify(reconnecting ? 'reconnecting' : 'connecting')
    source?.close()

    source = new EventSource(options.createUrl(cursor).toString())
    source.onopen = () => {
      if (!closed) {
        notify('live')
      }
    }

    source.onmessage = (message) => {
      if (closed) {
        return
      }

      try {
        const parse = options.parse ?? ((value: string) => JSON.parse(value) as T)
        const event = parse(message.data)
        const parsedLastEventId = Number.parseInt(message.lastEventId, 10)
        const nextCursor = options.getEventId?.(event) ?? (Number.isFinite(parsedLastEventId) ? parsedLastEventId : undefined)

        if (typeof nextCursor === 'number' && nextCursor > cursor) {
          cursor = nextCursor
        }

        options.onEvent(event)
        notify('live')
      } catch {
        // Ignore malformed stream frames so one bad event does not kill the connection.
      }
    }

    source.onerror = () => {
      if (closed) {
        return
      }

      source?.close()
      source = null
      notify('reconnecting')
      clearReconnect()
      reconnectTimer = window.setTimeout(() => connect(true), options.reconnectDelayMs ?? 1500)
    }
  }

  connect(false)

  return () => {
    closed = true
    clearReconnect()
    source?.close()
    source = null
    notify('closed')
  }
}
