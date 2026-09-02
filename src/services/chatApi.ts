const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? 'http://127.0.0.1:8080').replace(/\/$/, '')

export type ApiChannel = {
  id: string
  name: string
  group: string
  kind: 'channel' | 'dm'
  description?: string
  unread: number
  peer_user_id?: string
  presence?: 'online' | 'away' | 'offline'
  initials?: string
  color?: string
}

export type ApiUser = {
  id: string
  name: string
  email: string
  handle: string
  initials: string
  color: string
}

export type ApiMember = Omit<ApiUser, 'email'>

export type ApiChannelMember = ApiMember & {
  role: 'owner' | 'admin' | 'member'
  is_bot?: boolean
}

export type ApiReaction = {
  emoji: string
  count: number
  reacted?: boolean
}

export type ApiMessage = {
  id: string
  channel_id: string
  author_id?: string
  author: string
  initials: string
  color: string
  time: string
  body: string
  edited?: boolean
  deleted?: boolean
  reactions?: ApiReaction[]
  thread_count?: number
  parent_message_id?: string
}

export type ApiSummaryItem = {
  text: string
  source_message_id?: string
  source_parent_message_id?: string
}

export type ApiChannelSummary = {
  channel_id: string
  generated_at: string
  scope: 'unread' | 'recent'
  message_count: number
  unread_count: number
  summary: string
  decisions: ApiSummaryItem[]
  action_items: ApiSummaryItem[]
  unresolved: ApiSummaryItem[]
  chatter_count: number
  source_message_ids: string[]
}

export type RealtimeEvent = {
  type: 'message.created' | 'message.updated' | 'message.deleted' | 'reaction.added' | 'reaction.removed' | 'message.ai_started' | 'message.ai_delta' | 'message.ai_completed' | 'message.ai_failed' | 'typing.started' | 'typing.stopped' | 'presence.changed' | 'user.updated' | 'channel.created' | 'channel.updated' | 'channel.member_added' | 'channel.member_removed'
  channel_id: string
  event_id?: number
  sequence: number
  message?: ApiMessage
  message_id?: string
  parent_message_id?: string
  delta?: string
  error?: string
  actor_id?: string
  actor_name?: string
  actor_handle?: string
  actor_initials?: string
  actor_color?: string
  previous_actor_handle?: string
  presence?: 'online' | 'away' | 'offline'
  member_id?: string
}

export type MessagePage = {
  messages: ApiMessage[]
  next_cursor?: string
  has_more: boolean
  cursor: number
}

export type EventPage = {
  events: RealtimeEvent[]
  next_cursor?: string
  has_more: boolean
  cursor: number
}

export type ChannelResponse = { channels: ApiChannel[]; cursor: number }
type MemberResponse = { users: ApiMember[] }
type ChannelMemberResponse = { members: ApiChannelMember[] }
type UserResponse = { user: ApiUser }

export class ChatApiError extends Error {
  status: number
  code: string
  requestId?: string

  constructor(status: number, message: string, code = 'API_ERROR', requestId?: string) {
    super(message)
    this.name = 'ChatApiError'
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

export type ThreadRootPage = {
  messages: ApiMessage[]
  total: number
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })
  if (!response.ok) {
    const body = await response.text()
    let message = body || `API request failed: ${response.status}`
    let code = 'API_ERROR'
    let requestId: string | undefined
    try {
      const parsed = JSON.parse(body) as { error?: string; message?: string; code?: string; request_id?: string }
      if (parsed.error) message = parsed.error
      else if (parsed.message) message = parsed.message
      if (parsed.code) code = parsed.code
      requestId = parsed.request_id
    } catch {
      // Keep the raw response when the API does not return JSON.
    }
    throw new ChatApiError(response.status, message, code, requestId)
  }
  return response.json() as Promise<T>
}

export const chatApi = {
  async me() {
    const response = await request<UserResponse>('/api/auth/me')
    return response.user
  },

  async updateProfile(name: string) {
    const response = await request<UserResponse>('/api/auth/me', {
      method: 'PATCH',
      body: JSON.stringify({ name }),
    })
    return response.user
  },

  async register(payload: { name: string; email: string; password: string }) {
    const response = await request<UserResponse>('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
    return response.user
  },

  async login(payload: { email: string; password: string }) {
    const response = await request<UserResponse>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
    return response.user
  },

  async logout() {
    await request<{ status: string }>('/api/auth/logout', { method: 'POST' })
  },

  async listChannels() {
    return request<ChannelResponse>('/api/channels')
  },

  async listUsers() {
    return request<MemberResponse>('/api/users')
  },

  async listChannelMembers(channelId: string, signal?: AbortSignal) {
    return request<ChannelMemberResponse>(`/api/channels/${encodeURIComponent(channelId)}/members`, signal ? { signal } : undefined)
  },

  async createChannel(payload: Pick<ApiChannel, 'name' | 'group' | 'kind' | 'description'> & { member_ids?: string[] }) {
    return request<ApiChannel>('/api/channels', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  },

  async updateChannel(channelId: string, payload: { name: string; description: string; member_ids: string[] }) {
    return request<ApiChannel>(`/api/channels/${encodeURIComponent(channelId)}`, {
      method: 'PATCH',
      body: JSON.stringify(payload),
    })
  },

  async listMessages(channelId: string, before?: string, limit = 50, signal?: AbortSignal) {
    const params = new URLSearchParams({ limit: String(limit) })
    if (before) params.set('before', before)
    return request<MessagePage>(`/api/channels/${encodeURIComponent(channelId)}/messages?${params.toString()}`, signal ? { signal } : undefined)
  },

  async searchMessages(channelId: string, query: string, limit = 50, signal?: AbortSignal) {
    const params = new URLSearchParams({ q: query, limit: String(limit) })
    return request<MessagePage>(`/api/channels/${encodeURIComponent(channelId)}/messages?${params.toString()}`, signal ? { signal } : undefined)
  },

  async summarizeChannel(channelId: string, signal?: AbortSignal) {
    return request<ApiChannelSummary>(`/api/channels/${encodeURIComponent(channelId)}/summary`, {
      method: 'POST',
      signal,
    })
  },

  async listEvents(after: number, limit = 100) {
    const params = new URLSearchParams({ after: String(after), limit: String(limit) })
    return request<EventPage>(`/api/events?${params.toString()}`)
  },

  async markChannelRead(channelId: string) {
    return request<{ cursor: number; unread: number }>(`/api/channels/${encodeURIComponent(channelId)}/read`, {
      method: 'POST',
    })
  },

  async listThreadMessages(messageId: string, before?: string, limit = 50, signal?: AbortSignal) {
    const params = new URLSearchParams({ limit: String(limit) })
    if (before) params.set('before', before)
    return request<MessagePage>(`/api/messages/${encodeURIComponent(messageId)}/replies?${params.toString()}`, { signal })
  },

  async listThreadRoots(limit = 100, signal?: AbortSignal) {
    const params = new URLSearchParams({ limit: String(limit) })
    return request<ThreadRootPage>(`/api/threads?${params.toString()}`, signal ? { signal } : undefined)
  },

  async createMessage(channelId: string, payload: { body: string; parent_message_id?: string }) {
    return request<ApiMessage>(`/api/channels/${encodeURIComponent(channelId)}/messages`, {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  },

  async updateMessage(messageId: string, body: string) {
    return request<ApiMessage>(`/api/messages/${encodeURIComponent(messageId)}`, {
      method: 'PATCH',
      body: JSON.stringify({ body }),
    })
  },

  async deleteMessage(messageId: string) {
    return request<{ message_id: string }>(`/api/messages/${encodeURIComponent(messageId)}`, {
      method: 'DELETE',
    })
  },

  async addReaction(messageId: string, emoji: string) {
    return request<ApiMessage>(`/api/messages/${encodeURIComponent(messageId)}/reactions`, {
      method: 'POST',
      body: JSON.stringify({ emoji }),
    })
  },

  async removeReaction(messageId: string, emoji: string) {
    const params = new URLSearchParams({ emoji })
    return request<ApiMessage>(`/api/messages/${encodeURIComponent(messageId)}/reactions?${params.toString()}`, {
      method: 'DELETE',
    })
  },
}

type ConnectionStatus = 'connected' | 'reconnecting'

type SocketHandlers = {
  onEvent: (event: RealtimeEvent) => void
  onStatus: (status: ConnectionStatus) => void
}

export function createChatSocket(channelId: string, handlers: SocketHandlers) {
  const socketBaseUrl = API_BASE_URL.replace(/^http/, 'ws')
  let socket: WebSocket | null = null
  let retryTimer: number | undefined
  let closed = false
  let connectionGeneration = 0

  const connect = () => {
    if (closed) return
    const generation = connectionGeneration + 1
    connectionGeneration = generation
    handlers.onStatus('reconnecting')
    const nextSocket = new WebSocket(`${socketBaseUrl}/ws?channel_id=${encodeURIComponent(channelId)}`)
    socket = nextSocket
    nextSocket.onopen = () => {
      if (generation === connectionGeneration) handlers.onStatus('connected')
    }
    nextSocket.onmessage = (event) => {
      if (generation !== connectionGeneration) return
      try {
        handlers.onEvent(JSON.parse(event.data) as RealtimeEvent)
      } catch {
        // Ignore malformed events and keep the connection alive.
      }
    }
    nextSocket.onerror = () => nextSocket.close()
    nextSocket.onclose = () => {
      if (closed || generation !== connectionGeneration) return
      handlers.onStatus('reconnecting')
      retryTimer = window.setTimeout(connect, 1200)
    }
  }

  connect()

  return {
    send(payload: unknown) {
      if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify(payload))
    },
    reconnect() {
      if (closed) return
      if (retryTimer !== undefined) window.clearTimeout(retryTimer)
      retryTimer = undefined
      socket?.close()
      connect()
    },
    close() {
      closed = true
      connectionGeneration += 1
      if (retryTimer) window.clearTimeout(retryTimer)
      socket?.close()
    },
  }
}
