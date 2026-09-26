import { createElement } from 'react'
import { renderToString } from 'react-dom/server'
import { afterEach, expect, it, vi } from 'vitest'
import { chatApi } from '../services/chatApi'
import { useChatMessages } from './useChatMessages'

afterEach(() => vi.restoreAllMocks())

function harness(queue = Promise.resolve()) {
  let actions!: ReturnType<typeof useChatMessages>
  const setMessages = vi.fn()
  const setThreadReplies = vi.fn()
  function Harness() {
    actions = useChatMessages({
      backendReady: true,
      backendUnavailableMessage: 'offline',
      selectedChannelId: 'general',
      selectedChannelRef: { current: 'general' },
      messages: {},
      setMessages,
      setThreadReplies,
      setActionError: vi.fn(),
      advanceEventCursorRef: { current: vi.fn() },
      sendRealtime: vi.fn(),
      realtimeQueueRef: { current: queue },
    })
    return null
  }
  renderToString(createElement(Harness))
  return { actions, setMessages, setThreadReplies }
}

it('does not start a queued fetch after logout invalidates requests', async () => {
  let release!: () => void
  const queue = new Promise<void>((resolve) => { release = resolve })
  const fetch = vi.spyOn(chatApi, 'listMessages').mockResolvedValue({ messages: [], has_more: false, cursor: 0 })
  const { actions, setMessages } = harness(queue)
  const request = actions.loadMessages('general')
  actions.invalidateMessageRequests()
  release()
  await request
  expect(fetch).not.toHaveBeenCalled()
  expect(setMessages).not.toHaveBeenCalled()
})

it('does not update thread replies after logout during a deletion reload', async () => {
  let resolvePage!: (page: Awaited<ReturnType<typeof chatApi.listMessages>>) => void
  vi.spyOn(chatApi, 'deleteMessage').mockResolvedValue({ message_id: 'message-1' })
  const fetch = vi.spyOn(chatApi, 'listMessages').mockReturnValue(new Promise((resolve) => { resolvePage = resolve }))
  const { actions, setMessages, setThreadReplies } = harness()
  const request = actions.deleteMessage('message-1')
  await vi.waitFor(() => expect(fetch).toHaveBeenCalled())
  actions.invalidateMessageRequests()
  resolvePage({ messages: [], has_more: false, cursor: 0 })
  await request
  expect(setMessages).not.toHaveBeenCalled()
  expect(setThreadReplies).not.toHaveBeenCalled()
})

it('loads older pages until a saved message is found', async () => {
  const listMessages = vi.spyOn(chatApi, 'listMessages')
    .mockResolvedValueOnce({
      messages: [{ id: 'newer', channel_id: 'general', author: 'User', initials: 'U', color: '#fff', time: '', body: 'newer' }],
      has_more: true,
      next_cursor: '50',
      cursor: 100,
    })
    .mockResolvedValueOnce({
      messages: [{ id: 'saved', channel_id: 'general', author: 'User', initials: 'U', color: '#fff', time: '', body: 'saved' }],
      has_more: false,
      cursor: 100,
    })
  const { actions, setMessages } = harness()

  await expect(actions.loadMessageUntil('general', 'saved')).resolves.toBe(true)

  expect(listMessages).toHaveBeenNthCalledWith(1, 'general', undefined, 50, expect.any(AbortSignal))
  expect(listMessages).toHaveBeenNthCalledWith(2, 'general', '50', 50, expect.any(AbortSignal))
  expect(setMessages).toHaveBeenCalledTimes(2)
})
