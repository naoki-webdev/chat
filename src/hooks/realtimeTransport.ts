import { useCallback, useEffect, useRef } from 'react'
import { createChatSocket, type RealtimeEvent } from '../services/chatApi'

export type RealtimeConnection = 'connected' | 'reconnecting'

type Props = {
  enabled: boolean
  onStatus: (status: RealtimeConnection) => void
  onEvent: (event: RealtimeEvent) => void
}

export function useRealtimeTransport({ enabled, onStatus, onEvent }: Props) {
  const sendRef = useRef<(payload: unknown) => void>(() => undefined)
  const reconnectRef = useRef<() => void>(() => undefined)

  useEffect(() => {
    if (!enabled) {
      onStatus('reconnecting')
      return
    }
    const subscription = createChatSocket('all', { onStatus, onEvent })
    sendRef.current = subscription.send
    reconnectRef.current = subscription.reconnect
    return () => {
      sendRef.current = () => undefined
      reconnectRef.current = () => undefined
      subscription.close()
    }
  }, [enabled, onEvent, onStatus])

  const send = useCallback((payload: unknown) => sendRef.current(payload), [])
  const reconnect = useCallback(() => reconnectRef.current(), [])
  return { send, reconnect }
}
