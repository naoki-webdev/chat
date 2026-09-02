import { type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { type ApiUser, type RealtimeEvent } from '../services/chatApi'
import { fromApiMessage, mergeMessage, type Channel, type Message } from '../types/chat'
import { t } from '../i18n'

export type MessageMap = Record<string, Message[]>
export type TypingUsers = Record<string, Record<string, string>>

export type RealtimeEventHandlerOptions = {
  currentUserID: string
  setAuthUser: Dispatch<SetStateAction<ApiUser | null>>
  eventCursorRef: MutableRefObject<number>
  selectedChannelRef: MutableRefObject<string>
  threadRootRef: MutableRefObject<Message | null>
  refreshChannelsRef: MutableRefObject<(advanceCursor?: boolean) => Promise<void>>
  refreshSelectedChannelMembersRef: MutableRefObject<() => Promise<void>>
  scheduleSelectedChannelReadRef: MutableRefObject<(channelId: string) => void>
  setChannels: Dispatch<SetStateAction<Channel[]>>
  setMessages: Dispatch<SetStateAction<MessageMap>>
  setTypingUsers: Dispatch<SetStateAction<TypingUsers>>
  setMyPresence: Dispatch<SetStateAction<NonNullable<Channel['presence']>>>
  setThreadRoot: Dispatch<SetStateAction<Message | null>>
  setThreadReplies: Dispatch<SetStateAction<Message[]>>
  advanceEventCursor: (cursor: number) => void
  addThreadReply: (incoming: Message, channelId: string, countAsNew?: boolean) => void
}

export function createRealtimeEventHandler(options: RealtimeEventHandlerOptions) {
  const {
    currentUserID,
    setAuthUser,
    eventCursorRef,
    selectedChannelRef,
    threadRootRef,
    refreshChannelsRef,
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
  } = options

  return async (event: RealtimeEvent) => {
    const cursor = event.event_id ?? event.sequence
    // PostgreSQL emits message.created and message.ai_completed with the
    // same persisted sequence. The completion replaces the temporary
    // streaming message and must survive the cursor de-duplication.
    if (cursor > 0 && cursor <= eventCursorRef.current && !(event.type === 'message.ai_completed' && cursor === eventCursorRef.current)) return
    advanceEventCursor(cursor)

    if (event.type === 'channel.created' || event.type === 'channel.updated' || event.type === 'channel.member_added' || event.type === 'channel.member_removed') {
      await refreshChannelsRef.current()
      const currentUserWasRemoved = event.type === 'channel.member_removed' && event.member_id === currentUserID
      if (event.channel_id === selectedChannelRef.current && !currentUserWasRemoved) await refreshSelectedChannelMembersRef.current()
      return
    }

    if (event.type === 'typing.started' || event.type === 'typing.stopped') {
      if (!event.actor_id || event.actor_id === currentUserID || !event.actor_name) return
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
      if (event.actor_id === currentUserID) setMyPresence(event.presence)
      if (event.actor_handle) setChannels((current) => current.map((channel) => channel.kind === 'dm' && (channel.peerUserID === event.actor_id || channel.id === event.actor_handle)
        ? { ...channel, presence: event.presence }
        : channel))
      return
    }

    if (event.type === 'user.updated') {
      if (!event.actor_id || !event.actor_name || !event.actor_handle || !event.actor_initials || !event.actor_color) return
      const actorID = event.actor_id
      setAuthUser((user) => user?.id === actorID
        ? { ...user, name: event.actor_name!, handle: event.actor_handle!, initials: event.actor_initials!, color: event.actor_color! }
        : user)
      setMessages((current) => Object.fromEntries(Object.entries(current).map(([channelId, channelMessages]) => [
        channelId,
        channelMessages.map((message) => message.authorID === actorID
          ? { ...message, author: event.actor_name!, initials: event.actor_initials!, color: event.actor_color! }
          : message),
      ])))
      if (threadRootRef.current?.authorID === actorID) {
        const updatedRoot = { ...threadRootRef.current, author: event.actor_name, initials: event.actor_initials, color: event.actor_color }
        threadRootRef.current = updatedRoot
        setThreadRoot(updatedRoot)
      }
      setThreadReplies((current) => current.map((message) => message.authorID === actorID
        ? { ...message, author: event.actor_name!, initials: event.actor_initials!, color: event.actor_color! }
        : message))
      // DM channel IDs remain stable; only the display data follows a rename.
      setChannels((current) => setChannelsForUpdatedUser(current, event))
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
      if (event.channel_id === selectedChannelRef.current) {
        scheduleSelectedChannelReadRef.current(event.channel_id)
      } else if (incoming.authorID !== currentUserID) {
        setChannels((current) => current.map((channel) => channel.id === event.channel_id ? { ...channel, unread: channel.unread + 1 } : channel))
      }
      return
    }

    if (!event.message) return
    const incoming = fromApiMessage(event.message)
    const isMessageCreated = event.type === 'message.created'
    const isSelectedChannel = event.channel_id === selectedChannelRef.current
    const shouldIncrementUnread = isMessageCreated && !isSelectedChannel && incoming.authorID !== currentUserID
    if (isSelectedChannel && isMessageCreated) scheduleSelectedChannelReadRef.current(event.channel_id)
    if (incoming.parentMessageId) {
      addThreadReply(incoming, event.channel_id, isMessageCreated)
      if (shouldIncrementUnread) {
        setChannels((current) => current.map((channel) => channel.id === event.channel_id ? { ...channel, unread: channel.unread + 1 } : channel))
      }
      return
    }
    setMessages((current) => {
      const existing = current[event.channel_id] ?? []
      const index = existing.findIndex((message) => message.id === incoming.id)
      if (index < 0 && !isMessageCreated) return current
      const next = [...existing]
      if (index >= 0) next[index] = mergeMessage(existing[index], incoming)
      else next.push(incoming)
      return { ...current, [event.channel_id]: next }
    })
    if (shouldIncrementUnread) {
      setChannels((current) => current.map((channel) => channel.id === event.channel_id ? { ...channel, unread: channel.unread + 1 } : channel))
    }
  }
}

function setChannelsForUpdatedUser(channels: Channel[], event: RealtimeEvent) {
  const handles = new Set([event.previous_actor_handle, event.actor_handle].filter((handle): handle is string => Boolean(handle)))
  return channels.map((channel) => channel.kind === 'dm' && (channel.peerUserID === event.actor_id || handles.has(channel.id))
    ? { ...channel, name: event.actor_name!, initials: event.actor_initials!, color: event.actor_color! }
    : channel)
}
