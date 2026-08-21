import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { chatApi, ChatApiError } from './chatApi'

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    text: async () => JSON.stringify(body),
  }
}

describe('chatApi', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.clearAllMocks()
  })

  it('requests cursor-paginated messages with credentials', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ messages: [], has_more: true, next_cursor: '42', cursor: 43 }))

    const page = await chatApi.listMessages('general', '12', 20)

    expect(page.next_cursor).toBe('42')
    expect(fetchMock).toHaveBeenCalledWith(
      'http://127.0.0.1:8080/api/channels/general/messages?limit=20&before=12',
      expect.objectContaining({ credentials: 'include' }),
    )
  })

  it('loads event deltas and marks a channel as read', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ events: [], has_more: false, cursor: 9 }))
      .mockResolvedValueOnce(jsonResponse({ cursor: 9, unread: 0 }))

    await expect(chatApi.listEvents(8)).resolves.toMatchObject({ cursor: 9, events: [] })
    await expect(chatApi.markChannelRead('frontend')).resolves.toEqual({ cursor: 9, unread: 0 })
    expect(fetchMock.mock.calls[0][0]).toBe('http://127.0.0.1:8080/api/events?after=8&limit=100')
    expect(fetchMock.mock.calls[1][0]).toBe('http://127.0.0.1:8080/api/channels/frontend/read')
  })

  it('forwards an AbortSignal to summary requests', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ channel_id: 'general', scope: 'recent', message_count: 0, unread_count: 0, summary: '', decisions: [], action_items: [], unresolved: [], chatter_count: 0, source_message_ids: [], generated_at: '' }))
    const controller = new AbortController()

    await chatApi.summarizeChannel('general', controller.signal)

    expect(fetchMock).toHaveBeenCalledWith(
      'http://127.0.0.1:8080/api/channels/general/summary',
      expect.objectContaining({ credentials: 'include', signal: controller.signal, method: 'POST' }),
    )
  })

  it('converts API errors into ChatApiError', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ error: 'authentication required' }, 401))

    await expect(chatApi.me()).rejects.toEqual(expect.objectContaining<ChatApiError>({ name: 'ChatApiError', status: 401, message: 'authentication required' }))
  })

  it('uses the thread and reaction endpoints', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ messages: [], has_more: false, cursor: 5 }))
      .mockResolvedValueOnce(jsonResponse({ id: 'm-1', channel_id: 'general', author: 'Taro', initials: 'T', color: '#fff', time: '10:00', body: 'root' }))
      .mockResolvedValueOnce(jsonResponse({ id: 'm-1', channel_id: 'general', author: 'Taro', initials: 'T', color: '#fff', time: '10:00', body: 'root' }))
      .mockResolvedValueOnce(jsonResponse({ id: 'm-1', channel_id: 'general', author: 'Taro', initials: 'T', color: '#fff', time: '10:00', body: 'root' }))

    await chatApi.listThreadMessages('m-1')
    await chatApi.addReaction('m-1', '👍')
    await chatApi.removeReaction('m-1', '👍')

    expect(fetchMock.mock.calls[0][0]).toBe('http://127.0.0.1:8080/api/messages/m-1/replies?limit=50')
    expect(fetchMock.mock.calls[1][0]).toBe('http://127.0.0.1:8080/api/messages/m-1/reactions')
    expect(fetchMock.mock.calls[2][0]).toBe('http://127.0.0.1:8080/api/messages/m-1/reactions?emoji=%F0%9F%91%8D')
  })
})
