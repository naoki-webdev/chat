import { mergeMessage, type Message } from './chat'

export type MessageMap = Record<string, Message[]>

export function upsertMessageInMap(
  messages: MessageMap,
  channelId: string,
  incoming: Message,
  allowInsert = true,
): MessageMap {
  const existing = messages[channelId] ?? []
  const index = existing.findIndex((message) => message.id === incoming.id)
  if (index < 0 && !allowInsert) return messages

  const next = [...existing]
  if (index >= 0) next[index] = mergeMessage(existing[index], incoming)
  else next.push(incoming)
  return { ...messages, [channelId]: next }
}

export function replaceMessageInMap(messages: MessageMap, channelId: string, incoming: Message): MessageMap {
  const existing = messages[channelId] ?? []
  const index = existing.findIndex((message) => message.id === incoming.id)
  if (index < 0) return messages

  const next = [...existing]
  next[index] = incoming
  return { ...messages, [channelId]: next }
}

export function updateMessagesByAuthor(messages: MessageMap, authorId: string, update: Pick<Message, 'author' | 'initials' | 'color'>): MessageMap {
  return Object.fromEntries(Object.entries(messages).map(([channelId, channelMessages]) => [
    channelId,
    channelMessages.map((message) => message.authorID === authorId ? { ...message, ...update } : message),
  ]))
}
