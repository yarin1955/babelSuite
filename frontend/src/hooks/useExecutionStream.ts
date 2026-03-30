import { useEffect, useEffectEvent, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import {
  ApiError,
  getExecution,
  type ExecutionLogLine,
  type ExecutionLogStreamRecord,
  type ExecutionRecord,
  type ExecutionStreamEvent,
} from '../lib/api'
import {
  openExecutionEventStream,
  openExecutionLogStream,
} from '../lib/stream/events'
import type { StreamConnectionState } from '../lib/stream/sse'

interface UseExecutionStreamResult {
  execution: ExecutionRecord | null
  logs: ExecutionLogLine[]
  loading: boolean
  error: string
  paused: boolean
  setPaused: Dispatch<SetStateAction<boolean>>
  refresh: (silent?: boolean) => Promise<void>
  executionStreamState: StreamConnectionState
  logStreamState: StreamConnectionState
}

export function useExecutionStream(executionId: string): UseExecutionStreamResult {
  const [execution, setExecution] = useState<ExecutionRecord | null>(null)
  const [logs, setLogs] = useState<ExecutionLogLine[]>([])
  const [loading, setLoading] = useState(Boolean(executionId))
  const [error, setError] = useState('')
  const [paused, setPaused] = useState(false)
  const [executionStreamState, setExecutionStreamState] = useState<StreamConnectionState>('connecting')
  const [logStreamState, setLogStreamState] = useState<StreamConnectionState>('connecting')
  const executionCursorRef = useRef(0)
  const logCursorRef = useRef(0)

  const refresh = useEffectEvent(async (silent = false) => {
    const targetExecutionId = executionId
    if (!targetExecutionId) {
      setExecution(null)
      setLogs([])
      setLoading(false)
      setError('')
      return
    }

    if (!silent) {
      setLoading(true)
    }

    try {
      const nextExecution = await getExecution(targetExecutionId)
      if (targetExecutionId !== executionId) {
        return
      }

      executionCursorRef.current = Math.max(executionCursorRef.current, nextExecution.events.length)
      setExecution(nextExecution)
      setError('')
    } catch (reason) {
      if (targetExecutionId !== executionId) {
        return
      }

      const message = reason instanceof ApiError ? reason.message : 'Could not load execution.'
      setError(message)
    } finally {
      if (targetExecutionId === executionId) {
        setLoading(false)
      }
    }
  })

  const handleExecutionEvent = useEffectEvent((streamEvent: ExecutionStreamEvent) => {
    executionCursorRef.current = Math.max(executionCursorRef.current, streamEvent.id)
    setExecution((current) => mergeExecutionStreamEvent(current, streamEvent))
    setError('')
  })

  const handleExecutionState = useEffectEvent((state: StreamConnectionState) => {
    setExecutionStreamState(state)
    if (state === 'live') {
      setError('')
      return
    }
    if (state === 'reconnecting') {
      setError((current) => current || 'Execution event stream disconnected. Falling back to periodic refresh.')
    }
  })

  const handleLogRecord = useEffectEvent((record: ExecutionLogStreamRecord) => {
    logCursorRef.current = Math.max(logCursorRef.current, record.id)
    setLogs((current) => mergeLogRecord(current, record))
    setError('')
  })

  const handleLogState = useEffectEvent((state: StreamConnectionState) => {
    setLogStreamState(state)
    if (state === 'live') {
      setError('')
      return
    }
    if (state === 'reconnecting') {
      setError((current) => current || 'Terminal log stream disconnected. Visible output may pause until reconnect.')
    }
  })

  useEffect(() => {
    executionCursorRef.current = 0
    logCursorRef.current = 0
    setExecution(null)
    setLogs([])
    setPaused(false)
    setLoading(Boolean(executionId))
    setExecutionStreamState('connecting')
    setLogStreamState('connecting')
    void refresh()
  }, [executionId])

  useEffect(() => {
    if (!executionId || paused) {
      return
    }

    return openExecutionEventStream(executionId, {
      since: executionCursorRef.current,
      onEvent: handleExecutionEvent,
      onStateChange: handleExecutionState,
    })
  }, [executionId, paused])

  useEffect(() => {
    if (!executionId || paused) {
      return
    }

    return openExecutionLogStream(executionId, {
      since: logCursorRef.current,
      onEvent: handleLogRecord,
      onStateChange: handleLogState,
    })
  }, [executionId, paused])

  useEffect(() => {
    if (!executionId || paused) {
      return
    }

    const timer = window.setInterval(() => {
      void refresh(true)
    }, 8000)

    return () => window.clearInterval(timer)
  }, [executionId, paused])

  return {
    execution,
    logs,
    loading,
    error,
    paused,
    setPaused,
    refresh,
    executionStreamState,
    logStreamState,
  }
}

function mergeExecutionStreamEvent(current: ExecutionRecord | null, streamEvent: ExecutionStreamEvent) {
  if (!current || current.id !== streamEvent.executionId) {
    return current
  }

  const hasEvent = current.events.some((event) => event.id === streamEvent.event.id)
  return {
    ...current,
    status: streamEvent.executionStatus,
    duration: streamEvent.duration,
    updatedAt: streamEvent.updatedAt,
    events: hasEvent ? current.events : [...current.events, streamEvent.event],
  }
}

function mergeLogRecord(current: ExecutionLogLine[], record: ExecutionLogStreamRecord) {
  const duplicate = current.some((entry, index) => (
    index + 1 === record.id &&
    entry.source === record.line.source &&
    entry.timestamp === record.line.timestamp &&
    entry.text === record.line.text
  ))

  return duplicate ? current : [...current, record.line]
}
