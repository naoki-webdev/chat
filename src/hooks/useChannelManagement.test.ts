import { createElement } from 'react'
import { renderToString } from 'react-dom/server'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { chatApi, type ApiChannel } from '../services/chatApi'
import type { Channel, Message } from '../types/chat'
import { useChannelManagement } from './useChannelManagement'

afterEach(() => vi.restoreAllMocks())

describe('useChannelManagement', () => {
  it.each([false, true])('creates a channel without duplicating a prior realtime refresh (%s)', async (refreshed) => {
    const channel: ApiChannel = { id: 'new', name: 'New', group: 'Team', kind: 'channel', unread: 0 }
    let resolveCreate!: (channel: ApiChannel) => void
    vi.spyOn(chatApi, 'createChannel').mockReturnValue(new Promise((resolve) => { resolveCreate = resolve }))
    let channels: Channel[] = []
    let messages: Record<string, Message[]> = {}
    let actions!: ReturnType<typeof useChannelManagement>
    const select = vi.fn()
    function Harness() {
      actions = useChannelManagement({
        backendReady: true, backendUnavailableMessage: '', selectedChannelId: '',
        sessionVersion: 1, isSessionVersionActive: () => true, selectChannel: select,
        setChannels: (update) => { channels = typeof update === 'function' ? update(channels) : update },
        setMessages: (update) => { messages = typeof update === 'function' ? update(messages) : update },
        refreshSelectedChannelMembers: async () => {},
        setChannelCreateGroup: vi.fn(), setChannelEditOpen: vi.fn(), setActionError: vi.fn(),
      })
      return null
    }
    renderToString(createElement(Harness))
    const pending = actions.createChannel({ name: 'New', group: 'Team', description: '', memberIds: [] })
    const liveMessage: Message = { id: 'live', body: 'Already received', author: 'User', initials: 'U', color: '#fff', time: '' }
    if (refreshed) {
      channels = [{ ...channel, unread: 1 }]
      messages = { new: [liveMessage] }
    }
    resolveCreate(channel)
    await pending

    expect(channels).toHaveLength(1)
    expect(channels[0].unread).toBe(refreshed ? 1 : 0)
    expect(messages.new).toEqual(refreshed ? [liveMessage] : [])
    expect(select).toHaveBeenCalledWith(expect.objectContaining({ id: 'new' }))
  })

  it('ignores a channel creation response after the authenticated session changes', async () => {
    const channel: ApiChannel = { id: 'new', name: 'New', group: 'Team', kind: 'channel', unread: 0 }
    let resolveCreate!: (channel: ApiChannel) => void
    vi.spyOn(chatApi, 'createChannel').mockReturnValue(new Promise((resolve) => { resolveCreate = resolve }))
    const setChannels = vi.fn()
    const setMessages = vi.fn()
    const selectChannel = vi.fn()
    let activeVersion = 1
    let actions!: ReturnType<typeof useChannelManagement>
    function Harness() {
      actions = useChannelManagement({
        backendReady: true, backendUnavailableMessage: '', selectedChannelId: '', sessionVersion: 1,
        isSessionVersionActive: (version) => version === activeVersion,
        selectChannel,
        setChannels, setMessages, refreshSelectedChannelMembers: async () => {},
        setChannelCreateGroup: vi.fn(), setChannelEditOpen: vi.fn(), setActionError: vi.fn(),
      })
      return null
    }
    renderToString(createElement(Harness))
    const pending = actions.createChannel({ name: 'New', group: 'Team', description: '', memberIds: [] })
    activeVersion = 2
    resolveCreate(channel)
    await pending

    expect(setChannels).not.toHaveBeenCalled()
    expect(setMessages).not.toHaveBeenCalled()
    expect(selectChannel).not.toHaveBeenCalled()
  })
})
