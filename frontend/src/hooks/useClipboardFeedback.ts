import { useCallback, useEffect, useRef, useState } from 'react'

export function useClipboardFeedback(timeoutMs = 1800) {
  const [copiedId, setCopiedId] = useState('')
  const timeoutRef = useRef<number | null>(null)

  const clearTimer = useCallback(() => {
    if (timeoutRef.current !== null) {
      window.clearTimeout(timeoutRef.current)
      timeoutRef.current = null
    }
  }, [])

  const clearCopied = useCallback(() => {
    clearTimer()
    setCopiedId('')
  }, [clearTimer])

  const copyToClipboard = useCallback(async (id: string, value: string) => {
    await navigator.clipboard.writeText(value)
    clearTimer()
    setCopiedId(id)
    timeoutRef.current = window.setTimeout(() => {
      setCopiedId('')
      timeoutRef.current = null
    }, timeoutMs)
  }, [clearTimer, timeoutMs])

  useEffect(() => clearTimer, [clearTimer])

  return { copiedId, copyToClipboard, clearCopied }
}
