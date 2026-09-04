import { fromApiMessage, type Message } from '../types/chat'
import { replaceMessageInMap, type MessageMap, upsertMessageInMap } from '../types/messageState'
import type { RealtimeEvent } from '../services/chatApi'
import { t } from '../i18n'

export function reduceMessageEvent(messages: MessageMap, event: RealtimeEvent): MessageMap {
  if (event.type === 'message.deleted') {
    if (event.message) return replaceMessageInMap(messages, event.channel_id, fromApiMessage(event.message))
    if (!event.message_id) return messages
    const current = messages[event.channel_id] ?? []
    return {
      ...messages,
      [event.channel_id]: current
        .filter((message) => message.id !== event.message_id)
        .map((message) => event.parent_message_id === message.id
          ? { ...message, threadCount: Math.max(0, (message.threadCount ?? 0) - 1) }
          : message),
    }
  }

  if (event.type === 'message.ai_failed') {
    if (!event.message_id) return messages
    return updateMessageIfPresent(messages, event.channel_id, event.message_id, (message) => ({
      ...message,
      body: event.error ?? t('errors.aiFailed'),
      streaming: false,
      aiError: true,
    }))
  }

  if (event.type === 'message.ai_delta') {
    if (!event.message_id || !event.delta) return messages
    const current = messages[event.channel_id] ?? []
    const index = current.findIndex((message) => message.id === event.message_id)
    const next = [...current]
    if (index >= 0) next[index] = { ...next[index], body: next[index].body + event.delta, streaming: true }
    else next.push({ id: event.message_id, author: 'Orbit AI', initials: '✦', color: 'linear-gradient(135deg, #8b5cf6, #22d3ee)', time: '', body: event.delta, streaming: true })
    return { ...messages, [event.channel_id]: next }
  }

  if (event.type === 'message.ai_started') {
    if (!event.message) return messages
    const incoming = { ...fromApiMessage(event.message), streaming: true }
    if ((messages[event.channel_id] ?? []).some((message) => message.id === incoming.id)) return messages
    return { ...messages, [event.channel_id]: [...(messages[event.channel_id] ?? []), incoming] }
  }

  if (event.type === 'message.ai_completed') {
    if (!event.message) return messages
    const incoming = { ...fromApiMessage(event.message), streaming: false }
    const next = (messages[event.channel_id] ?? []).filter((message) => message.id !== event.message_id && message.id !== incoming.id)
    return { ...messages, [event.channel_id]: [...next, incoming] }
  }

  if (!event.message) return messages
  return upsertMessageInMap(messages, event.channel_id, fromApiMessage(event.message), event.type === 'message.created')
}

function updateMessageIfPresent(messages: MessageMap, channelId: string, messageId: string, update: (message: Message) => Message): MessageMap {
  const current = messages[channelId] ?? []
  if (!current.some((message) => message.id === messageId)) return messages
  return { ...messages, [channelId]: current.map((message) => message.id === messageId ? update(message) : message) }
}
