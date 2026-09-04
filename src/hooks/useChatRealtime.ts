import { useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi, type ApiUser, type RealtimeEvent } from '../services/chatApi'
import { createRealtimeEventHandler, type TypingUsers } from './realtimeEventHandler'
import { enqueueRealtimeTask, type RealtimeQueueRef } from './realtimeQueue'
import { useRealtimeSync } from './useRealtimeSync'
import { type Channel, type Message } from '../types/chat'
import { type MessageMap } from '../types/messageState'
import { useRealtimeTransport, type RealtimeConnection } from './realtimeTransport'

type UseChatRealtimeOptions = {
  enabled: boolean
  currentUser: ApiUser
  selectedChannelRef: MutableRefObject<string>
  threadRootRef: MutableRefObject<Message | null>
  threadReplyIDsRef: MutableRefObject<Set<string>>
  loadMessagesDirectRef: MutableRefObject<(channelId: string) => Promise<void>>
  realtimeQueueRef: RealtimeQueueRef
  refreshChannelsRef: MutableRefObject<(advanceCursor?: boolean) => Promise<void>>
  refreshSelectedChannelMembersRef: MutableRefObject<() => Promise<void>>
  setChannels: Dispatch<SetStateAction<Channel[]>>
  setAuthUser: Dispatch<SetStateAction<ApiUser | null>>
  setMessages: Dispatch<SetStateAction<MessageMap>>
  setTypingUsers: Dispatch<SetStateAction<TypingUsers>>
  setMyPresence: Dispatch<SetStateAction<NonNullable<Channel['presence']>>>
  setThreadRoot: Dispatch<SetStateAction<Message | null>>
  setThreadReplies: Dispatch<SetStateAction<Message[]>>
}

export function shouldApplyLiveRealtimeEvent(event: RealtimeEvent, currentCursor: number) {
  const cursor = event.event_id ?? event.sequence
  return cursor <= 0 || cursor > currentCursor || (event.type === 'message.ai_completed' && cursor === currentCursor)
}

export function useChatRealtime({
  enabled,
  currentUser,
  selectedChannelRef,
  threadRootRef,
  threadReplyIDsRef,
  loadMessagesDirectRef,
  realtimeQueueRef,
  refreshChannelsRef,
  refreshSelectedChannelMembersRef,
  setChannels,
  setAuthUser,
  setMessages,
  setTypingUsers,
  setMyPresence,
  setThreadRoot,
  setThreadReplies,
}: UseChatRealtimeOptions) {
  const [connection, setConnection] = useState<RealtimeConnection>('reconnecting')
  const eventCursorRef = useRef(0)
  const transportRef = useRef<{ reconnect: () => void }>({ reconnect: () => undefined })
  const refreshChannelsWithRetryRef = useRef<() => Promise<void>>(async () => undefined)
  const readTimersRef = useRef<Record<string, number>>({})
  const lastReconnectRequestRef = useRef(0)

  const advanceEventCursor = useCallback((cursor: number) => {
    if (cursor > eventCursorRef.current) eventCursorRef.current = cursor
  }, [])

  const listEvents = useCallback((cursor: number) => chatApi.listEvents(cursor), [])

  const requestReconnect = useCallback(() => {
    const now = Date.now()
    if (now - lastReconnectRequestRef.current < 1000) return
    lastReconnectRequestRef.current = now
    setConnection('reconnecting')
    transportRef.current.reconnect()
  }, [])

  const addThreadReply = useCallback((incoming: Message, channelId: string, countAsNew = false) => {
    if (!incoming.parentMessageId) return
    const replyKnown = threadReplyIDsRef.current.has(incoming.id)
    if (!countAsNew && !replyKnown) return
    threadReplyIDsRef.current.add(incoming.id)
    if (threadRootRef.current?.id === incoming.parentMessageId && selectedChannelRef.current === channelId) {
      setThreadReplies((current) => current.some((message) => message.id === incoming.id)
        ? current.map((message) => message.id === incoming.id ? incoming : message)
        : countAsNew ? [...current, incoming] : current)
    }
    setMessages((current) => ({
      ...current,
      [channelId]: (current[channelId] ?? []).map((message) => message.id === incoming.parentMessageId
        ? { ...message, threadCount: (message.threadCount ?? 0) + (countAsNew && !replyKnown ? 1 : 0) }
        : message),
    }))
  }, [selectedChannelRef, setMessages, setThreadReplies, threadReplyIDsRef, threadRootRef])

  const scheduleSelectedChannelRead = useCallback((channelId: string) => {
    const previousTimer = readTimersRef.current[channelId]
    if (previousTimer !== undefined) window.clearTimeout(previousTimer)
    readTimersRef.current[channelId] = window.setTimeout(() => {
      delete readTimersRef.current[channelId]
      void chatApi.markChannelRead(channelId).catch(() => undefined)
    }, 350)
  }, [])
  const scheduleSelectedChannelReadRef = useRef(scheduleSelectedChannelRead)
  scheduleSelectedChannelReadRef.current = scheduleSelectedChannelRead

  const refreshChannelsWithRetry = useCallback(async () => {
    let lastError: unknown
    for (const delay of [0, 250, 500]) {
      if (delay > 0) await new Promise<void>((resolve) => window.setTimeout(resolve, delay))
      try {
        await refreshChannelsRef.current()
        setConnection('connected')
        return
      } catch (error) {
        lastError = error
      }
    }
    setConnection('reconnecting')
    requestReconnect()
    throw lastError instanceof Error ? lastError : new Error('channel refresh failed')
  }, [refreshChannelsRef, requestReconnect])
  refreshChannelsWithRetryRef.current = refreshChannelsWithRetry

  const applyRealtimeEvent = useMemo(() => createRealtimeEventHandler({
      currentUserID: currentUser.id,
      setAuthUser,
      eventCursorRef,
      selectedChannelRef,
      threadRootRef,
      refreshChannelsRef: refreshChannelsWithRetryRef,
      refreshSelectedChannelMembersRef,
      scheduleSelectedChannelReadRef,
      setChannels,
      setMessages,
      setTypingUsers,
      setMyPresence,
      setThreadRoot,
      setThreadReplies,
      advanceEventCursor,
      addThreadReply,
    }), [addThreadReply, advanceEventCursor, currentUser.id, refreshChannelsWithRetryRef, refreshSelectedChannelMembersRef, setAuthUser, setChannels, setMessages, setMyPresence, setThreadReplies, setThreadRoot, setTypingUsers, threadRootRef, selectedChannelRef])

  const enqueueRealtimeEvent = useCallback((event: RealtimeEvent) => {
    void enqueueRealtimeTask(realtimeQueueRef, async () => {
      if (!shouldApplyLiveRealtimeEvent(event, eventCursorRef.current)) return
      // sequence is shared by every channel. Events from channels the user
      // cannot access therefore create expected gaps in the visible stream.
      // Catch-up is handled on reconnect; a live gap must not trigger a
      // redundant sync request.
      await applyRealtimeEvent(event)
    }).catch(() => requestReconnect())
	}, [applyRealtimeEvent, realtimeQueueRef, requestReconnect])

  const { enqueueEventSync } = useRealtimeSync({
    realtimeQueueRef,
    eventCursorRef,
    selectedChannelRef,
    loadMessagesDirectRef,
    refreshChannelsWithRetryRef,
    refreshSelectedChannelMembersRef,
    listEvents,
    onEvent: applyRealtimeEvent,
    onCursor: advanceEventCursor,
    requestReconnect,
  })

  const onTransportStatus = useCallback((status: RealtimeConnection) => {
    setConnection(status)
    if (status === 'connected') enqueueEventSync()
  }, [enqueueEventSync])
  const transport = useRealtimeTransport({ enabled, onStatus: onTransportStatus, onEvent: enqueueRealtimeEvent })
  transportRef.current = transport

  useEffect(() => () => {
    Object.values(readTimersRef.current).forEach((timer) => window.clearTimeout(timer))
    readTimersRef.current = {}
  }, [])

  return { connection, send: transport.send, addThreadReply, advanceEventCursor }
}
