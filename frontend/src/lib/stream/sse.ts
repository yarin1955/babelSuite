export type StreamConnectionState = 'connecting' | 'live' | 'reconnecting' | 'closed'

interface ReplayableEventStreamOptions<T> {
  initialCursor?: number
  createUrl: (cursor: number) => URL
  getToken: () => string | undefined
  onEvent: (event: T) => void
  onStateChange?: (state: StreamConnectionState) => void
  getEventId?: (event: T) => number | undefined
  reconnectDelayMs?: number
  parse?: (data: string) => T
}

function parseSseChunk(chunk: string): { data: string; id: string } | null {
  let data = ''
  let id = ''
  for (const line of chunk.split('\n')) {
    if (line.startsWith('data:')) data = line.slice(5).trim()
    else if (line.startsWith('id:')) id = line.slice(3).trim()
  }
  return data ? { data, id } : null
}

export function openReplayableEventStream<T>(options: ReplayableEventStreamOptions<T>) {
  let abortController: AbortController | null = null
  let reconnectTimer: number | null = null
  let closed = false
  let cursor = options.initialCursor ?? 0

  const notify = (state: StreamConnectionState) => options.onStateChange?.(state)

  const clearReconnect = () => {
    if (reconnectTimer !== null) {
      window.clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  const connect = async (reconnecting: boolean) => {
    if (closed) return

    notify(reconnecting ? 'reconnecting' : 'connecting')
    abortController?.abort()
    abortController = new AbortController()

    const url = options.createUrl(cursor)
    const headers: Record<string, string> = {
      Accept: 'text/event-stream',
      'Cache-Control': 'no-cache',
    }
    const token = options.getToken()
    if (token) headers['Authorization'] = `Bearer ${token}`
    if (cursor > 0) headers['Last-Event-ID'] = String(cursor)

    try {
      const response = await fetch(url.toString(), {
        headers,
        signal: abortController.signal,
      })

      if (!response.ok || !response.body) {
        throw new Error(`SSE connect failed: ${response.status}`)
      }

      notify('live')

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done || closed) break

        buffer += decoder.decode(value, { stream: true })
        const events = buffer.split('\n\n')
        buffer = events.pop() ?? ''

        for (const chunk of events) {
          if (!chunk.trim()) continue
          const parsed = parseSseChunk(chunk)
          if (!parsed) continue

          try {
            const parse = options.parse ?? ((s: string) => JSON.parse(s) as T)
            const event = parse(parsed.data)
            const idFromEvent = options.getEventId?.(event)
            const idFromFrame = parsed.id ? Number.parseInt(parsed.id, 10) : NaN
            const nextCursor = idFromEvent ?? (Number.isFinite(idFromFrame) ? idFromFrame : undefined)
            if (typeof nextCursor === 'number' && nextCursor > cursor) {
              cursor = nextCursor
            }
            options.onEvent(event)
            notify('live')
          } catch {
            // Ignore malformed SSE frames so one bad event does not kill the connection.
          }
        }
      }
    } catch (err: unknown) {
      if (closed || (err instanceof DOMException && err.name === 'AbortError')) return
      clearReconnect()
      reconnectTimer = window.setTimeout(() => void connect(true), options.reconnectDelayMs ?? 1500)
    }
  }

  void connect(false)

  return () => {
    closed = true
    clearReconnect()
    abortController?.abort()
    abortController = null
    notify('closed')
  }
}
