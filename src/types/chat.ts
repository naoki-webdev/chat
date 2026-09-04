import type { ApiChannel, ApiMessage } from '../services/chatApi'

export type Channel = {
  id: string
  name: string
  group: string
  kind: 'channel' | 'dm'
  unread: number
  peerUserID?: string
  description?: string
  presence?: 'online' | 'away' | 'offline'
  initials?: string
  color?: string
}
export type Reaction = { emoji: string; count: number; reacted?: boolean }

export type Message = {
  id: string
  authorID?: string
  author: string
  initials: string
  color: string
  time: string
  body: string
  edited?: boolean
  deleted?: boolean
  reactions?: Reaction[]
  threadCount?: number
  parentMessageId?: string
  streaming?: boolean
  aiError?: boolean
}

export function fromApiChannel(channel: ApiChannel): Channel {
  return {
    id: channel.id,
    name: channel.name,
    group: channel.group,
    kind: channel.kind,
    unread: channel.unread,
    peerUserID: channel.peer_user_id,
    description: channel.description,
    presence: channel.presence,
    initials: channel.initials,
    color: channel.color,
  }
}

export function fromApiMessage(message: ApiMessage): Message {
  return {
    id: message.id,
    authorID: message.author_id,
    author: message.author,
    initials: message.initials,
    color: message.color,
    time: message.time,
    body: message.body,
    edited: message.edited,
    deleted: message.deleted,
    reactions: message.reactions,
    threadCount: message.thread_count,
    parentMessageId: message.parent_message_id,
  }
}

export function mergeMessage(existing: Message | undefined, incoming: Message): Message {
  if (!existing) return incoming

  const existingReactions = new Map((existing.reactions ?? []).map((reaction) => [reaction.emoji, reaction]))
  return {
    ...incoming,
    authorID: incoming.authorID ?? existing.authorID,
    reactions: incoming.reactions?.map((reaction) => {
      if (reaction.reacted !== undefined) return reaction
      const previous = existingReactions.get(reaction.emoji)
      return previous?.reacted === undefined ? reaction : { ...reaction, reacted: previous.reacted }
    }),
  }
}
