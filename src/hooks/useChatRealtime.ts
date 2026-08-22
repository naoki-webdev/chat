import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi, createChatSocket, type ApiUser, type RealtimeEvent } from '../services/chatApi'
import { createRealtimeEventHandler, type MessageMap, type TypingUsers } from './realtimeEventHandler'
import { type Channel, type Message } from '../types/chat'

type UseChatRealtimeOptions = {
  enabled: boolean
  currentUser: ApiUser
  selectedChannelRef: MutableRefObject<string>
  threadRootRef: MutableRefObject<Message | null>
  threadReplyIDsRef: MutableRefObject<Set<string>>
  loadMessagesRef: MutableRefObject<(channelId: string) => Promise<void>>
  refreshChannelsRef: MutableRefObject<(advanceCursor?: boolean) => Promise<void>>
  refreshSelectedChannelMembersRef: MutableRefObject<() => Promise<void>>
  setChannels: Dispatch<SetStateAction<Channel[]>>
  setMessages: Dispatch<SetStateAction<MessageMap>>
  setTypingUsers: Dispatch<SetStateAction<TypingUsers>>
  setMyPresence: Dispatch<SetStateAction<NonNullable<Channel['presence']>>>
  setThreadRoot: Dispatch<SetStateAction<Message | null>>
  setThreadReplies: Dispatch<SetStateAction<Message[]>>
}

export function useChatRealtime({
  enabled,
  currentUser,
  selectedChannelRef,
  threadRootRef,
  threadReplyIDsRef,
  loadMessagesRef,
  refreshChannelsRef,
  refreshSelectedChannelMembersRef,
  setChannels,
  setMessages,
  setTypingUsers,
  setMyPresence,
  setThreadRoot,
  setThreadReplies,
}: UseChatRealtimeOptions) {
  const [connection, setConnection] = useState<'connected' | 'reconnecting'>('reconnecting')
  const eventCursorRef = useRef(0)
  const realtimeQueueRef = useRef<Promise<void>>(Promise.resolve())
  const socketSendRef = useRef<(payload: unknown) => void>(() => undefined)

  const advanceEventCursor = useCallback((cursor: number) => {
    if (cursor > eventCursorRef.current) eventCursorRef.current = cursor
  }, [])

  const addThreadReply = useCallback((incoming: Message, channelId: string, countAsNew = false) => {
    if (!incoming.parentMessageId) return
    const replyKnown = threadReplyIDsRef.current.has(incoming.id)
    threadReplyIDsRef.current.add(incoming.id)
    if (threadRootRef.current?.id === incoming.parentMessageId && selectedChannelRef.current === channelId) {
      setThreadReplies((current) => current.some((message) => message.id === incoming.id)
        ? current.map((message) => message.id === incoming.id ? incoming : message)
        : [...current, incoming])
    }
    setMessages((current) => ({
      ...current,
      [channelId]: (current[channelId] ?? []).map((message) => message.id === incoming.parentMessageId
        ? { ...message, threadCount: (message.threadCount ?? 0) + (countAsNew && !replyKnown ? 1 : 0) }
        : message),
    }))
  }, [selectedChannelRef, setMessages, setThreadReplies, threadReplyIDsRef, threadRootRef])

  const applyRealtimeEvent = useCallback(
    createRealtimeEventHandler({
      currentUserID: currentUser.id,
      eventCursorRef,
      selectedChannelRef,
      threadRootRef,
      threadReplyIDsRef,
      refreshChannelsRef,
      refreshSelectedChannelMembersRef,
      setChannels,
      setMessages,
      setTypingUsers,
      setMyPresence,
      setThreadRoot,
      setThreadReplies,
      advanceEventCursor,
      addThreadReply,
    }),
    [addThreadReply, advanceEventCursor, currentUser.id, refreshChannelsRef, refreshSelectedChannelMembersRef, setChannels, setMessages, setMyPresence, setThreadReplies, setThreadRoot, setTypingUsers, threadRootRef, selectedChannelRef],
  )

  const syncEvents = useCallback(async (after: number) => {
    let cursor = after
    for (let pageNumber = 0; pageNumber < 100; pageNumber += 1) {
      const page = await chatApi.listEvents(cursor)
      page.events.forEach(applyRealtimeEvent)
      if (!page.has_more) {
        advanceEventCursor(page.cursor)
        return
      }
      const nextCursor = Number(page.next_cursor)
      if (!Number.isFinite(nextCursor) || nextCursor <= cursor) return
      cursor = nextCursor
    }
  }, [advanceEventCursor, applyRealtimeEvent])

  const enqueueRealtimeEvent = useCallback((event: RealtimeEvent) => {
    realtimeQueueRef.current = realtimeQueueRef.current.then(async () => {
      const cursor = event.event_id ?? event.sequence
      if (cursor <= 0) {
        applyRealtimeEvent(event)
        return
      }
      if (cursor <= eventCursorRef.current) return
      if (cursor > eventCursorRef.current + 1) await syncEvents(eventCursorRef.current)
      if (cursor > eventCursorRef.current) applyRealtimeEvent(event)
    }).catch(() => undefined)
  }, [applyRealtimeEvent, syncEvents])

  const enqueueEventSync = useCallback(() => {
    realtimeQueueRef.current = realtimeQueueRef.current.then(async () => {
      await syncEvents(eventCursorRef.current)
      await refreshChannelsRef.current()
      await loadMessagesRef.current(selectedChannelRef.current)
    }).catch(() => undefined)
  }, [loadMessagesRef, selectedChannelRef, syncEvents])

  useEffect(() => {
    if (!enabled) {
      setConnection('reconnecting')
      return
    }
    const subscription = createChatSocket('all', {
      onStatus: (status) => {
        setConnection(status)
        if (status === 'connected') enqueueEventSync()
      },
      onEvent: enqueueRealtimeEvent,
    })
    socketSendRef.current = subscription.send
    return () => {
      socketSendRef.current = () => undefined
      subscription.close()
    }
  }, [enabled, enqueueEventSync, enqueueRealtimeEvent, currentUser.name, currentUser.handle, currentUser.initials, currentUser.color])

  const send = useCallback((payload: unknown) => socketSendRef.current(payload), [])

  return { connection, send, addThreadReply, advanceEventCursor }
}
