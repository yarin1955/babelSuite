import { startTransition, useEffect, useEffectEvent, useRef, useState } from 'react'
import { ApiError, getSandboxes, type SandboxesResponse } from '../lib/api'
import { openSandboxEventStream } from '../lib/stream/events'
import type { StreamConnectionState } from '../lib/stream/sse'

interface UseSandboxStreamResult {
  snapshot: SandboxesResponse | null
  loading: boolean
  refreshing: boolean
  error: string
  streamState: StreamConnectionState
  refresh: (silent?: boolean) => Promise<void>
}

export function useSandboxStream(): UseSandboxStreamResult {
  const [snapshot, setSnapshot] = useState<SandboxesResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState('')
  const [streamState, setStreamState] = useState<StreamConnectionState>('connecting')
  const lastStreamEventId = useRef(0)

  const applySnapshot = useEffectEvent((response: SandboxesResponse) => {
    startTransition(() => {
      setSnapshot(response)
      setLoading(false)
      setRefreshing(false)
    })
    setError('')
  })

  const refresh = useEffectEvent(async (silent = false) => {
    if (silent) {
      setRefreshing(true)
    } else {
      setLoading(true)
    }

    try {
      const response = await getSandboxes()
      applySnapshot(response)
    } catch (reason) {
      const message = reason instanceof ApiError ? reason.message : 'Could not load the active sandbox inventory.'
      setError(message)
      setLoading(false)
      setRefreshing(false)
    }
  })

  const handleStreamState = useEffectEvent((state: StreamConnectionState) => {
    setStreamState(state)
    if (state === 'live') {
      setError('')
      return
    }
    if (state === 'reconnecting') {
      setError((current) => current || 'Live sandbox stream disconnected. Showing the last known inventory until reconnect.')
    }
  })

  useEffect(() => {
    void refresh()
  }, [])

  useEffect(() => openSandboxEventStream({
    since: lastStreamEventId.current,
    onEvent: (event) => {
      lastStreamEventId.current = Math.max(lastStreamEventId.current, event.id)
      applySnapshot(event.snapshot)
    },
    onStateChange: handleStreamState,
  }), [])

  return {
    snapshot,
    loading,
    refreshing,
    error,
    streamState,
    refresh,
  }
}
