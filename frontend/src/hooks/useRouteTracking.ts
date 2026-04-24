import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router-dom'
import { SpanStatusCode } from '@opentelemetry/api'
import { getTracer, navigationCounter } from '../lib/telemetry'

export function useRouteTracking() {
  const location = useLocation()
  const prevPathRef = useRef<string | null>(null)

  useEffect(() => {
    const path = location.pathname
    if (prevPathRef.current === path) return
    const previous = prevPathRef.current
    prevPathRef.current = path

    navigationCounter.add(1, { 'navigation.route': path })

    getTracer().startActiveSpan('navigation', { attributes: {
      'navigation.route': path,
      ...(previous != null ? { 'navigation.from': previous } : {}),
    }}, (span) => {
      span.setStatus({ code: SpanStatusCode.OK })
      span.end()
    })
  }, [location.pathname])
}
