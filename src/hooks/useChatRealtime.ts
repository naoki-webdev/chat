import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi, createChatSocket, type ApiUser, type RealtimeEvent } from '../services/chatApi'
import { fromApiMessage, mergeMessage, type Channel, type Message } from '../types/chat'
import { t } from '../i18n'

type MessageMap = Record<string, Message[]>
type TypingUsers = Record<string, Record<string, string>>

type UseChatRealtimeOptions = {
  enabled: boolean
  currentUser: ApiUser
  selectedChannelRef: MutableRefObject<string>
  threadRootRef: MutableRefObject<Message | null>
  threadReplyIDsRef: MutableRefObject<Set<string>>
  loadMessagesRef: MutableRefObject<(channelId: string) => Promise<void>>
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

  const applyRealtimeEvent = useCallback((event: RealtimeEvent) => {
    const cursor = event.event_id ?? event.sequence
    if (cursor > 0 && cursor <= eventCursorRef.current) return
    advanceEventCursor(cursor)

    if (event.type === 'typing.started' || event.type === 'typing.stopped') {
      if (!event.actor_id || event.actor_id === currentUser.id || !event.actor_name) return
      const actorID = event.actor_id
      const actorName = event.actor_name
      setTypingUsers((current) => {
        const channelTyping = { ...(current[event.channel_id] ?? {}) }
        if (event.type === 'typing.started') channelTyping[actorID] = actorName
        else delete channelTyping[actorID]
        return { ...current, [event.channel_id]: channelTyping }
      })
      return
    }

    if (event.type === 'presence.changed') {
      if (!event.actor_id || !event.presence) return
      if (event.actor_id === currentUser.id) setMyPresence(event.presence)
      if (event.actor_handle) setChannels((current) => current.map((channel) => channel.id === event.actor_handle ? { ...channel, presence: event.presence } : channel))
      return
    }

    if (event.type === 'message.deleted') {
      const deletedMessageID = event.message_id
      if (event.message) {
        const incoming = fromApiMessage(event.message)
        setMessages((current) => ({
          ...current,
          [event.channel_id]: (current[event.channel_id] ?? []).map((message) => message.id === incoming.id ? incoming : message),
        }))
        if (threadRootRef.current?.id === incoming.id) {
          threadRootRef.current = incoming
          setThreadRoot(incoming)
        }
      } else if (deletedMessageID) {
        setMessages((current) => ({
          ...current,
          [event.channel_id]: (current[event.channel_id] ?? [])
            .filter((message) => message.id !== deletedMessageID)
            .map((message) => event.parent_message_id === message.id
              ? { ...message, threadCount: Math.max(0, (message.threadCount ?? 0) - 1) }
              : message),
        }))
        setThreadReplies((current) => current.filter((message) => message.id !== deletedMessageID))
      }
      return
    }

    if (event.type === 'message.ai_failed') {
      if (!event.message_id) return
      setMessages((current) => ({
        ...current,
        [event.channel_id]: (current[event.channel_id] ?? []).map((message) => message.id === event.message_id
          ? { ...message, body: event.error ?? t('errors.aiFailed'), streaming: false, aiError: true }
          : message),
      }))
      return
    }

    if (event.type === 'message.ai_delta') {
      if (!event.message_id || !event.delta) return
      const messageID = event.message_id
      const delta = event.delta
      setMessages((current) => {
        const existing = current[event.channel_id] ?? []
        const index = existing.findIndex((message) => message.id === messageID)
        const next = [...existing]
        if (index >= 0) next[index] = { ...next[index], body: next[index].body + delta, streaming: true }
        else next.push({ id: messageID, author: 'Orbit AI', initials: '✦', color: 'linear-gradient(135deg, #8b5cf6, #22d3ee)', time: '', body: delta, streaming: true })
        return { ...current, [event.channel_id]: next }
      })
      return
    }

    if (event.type === 'message.ai_started') {
      if (!event.message) return
      const incoming = { ...fromApiMessage(event.message), streaming: true }
      setMessages((current) => {
        const existing = current[event.channel_id] ?? []
        if (existing.some((message) => message.id === incoming.id)) return current
        return { ...current, [event.channel_id]: [...existing, incoming] }
      })
      return
    }

    if (event.type === 'message.ai_completed') {
      if (!event.message) return
      const incoming = { ...fromApiMessage(event.message), streaming: false }
      setMessages((current) => {
        const existing = current[event.channel_id] ?? []
        const next = existing.filter((message) => message.id !== event.message_id && message.id !== incoming.id)
        return { ...current, [event.channel_id]: [...next, incoming] }
      })
      if (event.channel_id !== selectedChannelRef.current) {
        setChannels((current) => current.map((channel) => channel.id === event.channel_id ? { ...channel, unread: channel.unread + 1 } : channel))
      }
      return
    }

    if (!event.message) return
    const incoming = fromApiMessage(event.message)
    if (incoming.parentMessageId) {
      addThreadReply(incoming, event.channel_id, event.type === 'message.created')
      return
    }
    setMessages((current) => {
      const existing = current[event.channel_id] ?? []
      const index = existing.findIndex((message) => message.id === incoming.id)
      const next = [...existing]
      if (index >= 0) next[index] = mergeMessage(existing[index], incoming)
      else next.push(incoming)
      return { ...current, [event.channel_id]: next }
    })
    if (event.channel_id !== selectedChannelRef.current && event.type === 'message.created') {
      setChannels((current) => current.map((channel) => channel.id === event.channel_id ? { ...channel, unread: channel.unread + 1 } : channel))
    }
  }, [addThreadReply, advanceEventCursor, currentUser.id, setChannels, setMessages, setMyPresence, setThreadReplies, setThreadRoot, setTypingUsers, threadRootRef, selectedChannelRef])

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
