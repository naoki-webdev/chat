import { createElement } from 'react'
import { renderToString } from 'react-dom/server'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { chatApi } from '../services/chatApi'
import type { Message } from '../types/chat'
import { demoUser, initialChannels, initialMessages } from '../types/demoData'
import { useWorkspaceActions } from './useWorkspaceActions'

afterEach(() => vi.restoreAllMocks())

describe('useWorkspaceActions', () => {
  it('clears channel-local state when changing channels and logging out', async () => {
    vi.spyOn(chatApi, 'logout').mockResolvedValue(undefined)

    const selectedChannelRef = { current: 'frontend' }
    const threadRootRef: { current: Message | null } = { current: null }
    const threadReplyIDsRef = { current: new Set(['reply']) }
    const setDraft = vi.fn()
    const setEditingId = vi.fn()
    const setEditDraft = vi.fn()
    const setSearchOpen = vi.fn()
    const setSearchQuery = vi.fn()
    const setWorkspaceOverlay = vi.fn()
    const setChannelEditOpen = vi.fn()
    const setChannelCreateGroup = vi.fn()
    const setAvailableMembers = vi.fn()
    const setAvailableMembersLoaded = vi.fn()
    const setChannels = vi.fn()
    const setMessages = vi.fn()
    const setAuthUser = vi.fn()
    const setAuthState = vi.fn()
    const setBackendState = vi.fn()
    const setMyPresence = vi.fn()
    const setTypingUsers = vi.fn()
    const setThreadDraft = vi.fn()
    const invalidateMessageRequests = vi.fn()
    const invalidateChannelRefresh = vi.fn()
    const clearScheduledChannelReads = vi.fn()
    const setThreadRoot = vi.fn()
    const setThreadReplies = vi.fn()
    const setActionError = vi.fn()
    const stopTyping = vi.fn()
    const invalidateThreadRequest = vi.fn()
    let actions!: ReturnType<typeof useWorkspaceActions>

    function Harness() {
      actions = useWorkspaceActions({
        backendReady: false,
        backendUnavailableMessage: 'offline',
        currentUser: demoUser,
        selectedChannelRef,
        threadRootRef,
        threadReplyIDsRef,
        invalidateMessageRequests,
        invalidateChannelRefresh,
        clearScheduledChannelReads,
        stopTyping,
        invalidateThreadRequest,
        sendPresence: vi.fn(),
        setSelectedChannelId: vi.fn(),
        setDraft,
        setEditingId,
        setEditDraft,
        setSearchOpen,
        setSearchQuery,
        setWorkspaceOverlay,
        setChannelEditOpen,
        setChannelCreateGroup,
        setAvailableMembers,
        setAvailableMembersLoaded,
        setChannels,
        setMessages,
        setAuthUser,
        setAuthState,
        setBackendState,
        setMyPresence,
        setTypingUsers,
        setThreadDraft,
        setThreadRoot,
        setThreadReplies,
        setActionError,
      })
      return null
    }

    renderToString(createElement(Harness))

    actions.selectChannel(initialChannels[0])
    expect(setDraft).toHaveBeenCalledWith('')
    expect(setEditingId).toHaveBeenCalledWith(null)
    expect(setEditDraft).toHaveBeenCalledWith('')
    expect(setThreadDraft).toHaveBeenCalledWith('')

    await actions.logout()
    expect(invalidateMessageRequests).toHaveBeenCalled()
    expect(invalidateChannelRefresh).toHaveBeenCalled()
    expect(clearScheduledChannelReads).toHaveBeenCalled()
    expect(setChannelCreateGroup).toHaveBeenCalledWith(null)
    expect(setAvailableMembers).toHaveBeenCalledWith([])
    expect(setAvailableMembersLoaded).toHaveBeenCalledWith(false)
    expect(setDraft).toHaveBeenLastCalledWith('')
    expect(setChannels).toHaveBeenCalledWith(initialChannels)
    expect(setMessages).toHaveBeenCalledWith(initialMessages)
    expect(setSearchOpen).toHaveBeenCalledWith(false)
    expect(setWorkspaceOverlay).toHaveBeenCalledWith(null)
    expect(setTypingUsers).toHaveBeenCalledWith({})
    expect(setAuthUser).toHaveBeenCalledWith(null)
    expect(setAuthState).toHaveBeenCalledWith('anonymous')
    expect(setBackendState).toHaveBeenCalledWith('checking')
    expect(selectedChannelRef.current).toBe('design-system')
    expect(threadRootRef.current).toBeNull()
    expect(threadReplyIDsRef.current.size).toBe(0)
  })

  it('keeps the authenticated workspace when logout fails', async () => {
    vi.spyOn(chatApi, 'logout').mockRejectedValue(new Error('network unavailable'))
    const setAuthUser = vi.fn()
    const setAuthState = vi.fn()
    const setChannels = vi.fn()
    const setActionError = vi.fn()
    let actions!: ReturnType<typeof useWorkspaceActions>
    function Harness() {
      actions = useWorkspaceActions({
        backendReady: true, backendUnavailableMessage: 'offline', currentUser: demoUser,
        selectedChannelRef: { current: 'general' }, threadRootRef: { current: null }, threadReplyIDsRef: { current: new Set() },
        invalidateMessageRequests: vi.fn(), invalidateChannelRefresh: vi.fn(), clearScheduledChannelReads: vi.fn(),
        stopTyping: vi.fn(), invalidateThreadRequest: vi.fn(), sendPresence: vi.fn(), setSelectedChannelId: vi.fn(),
        setDraft: vi.fn(), setEditingId: vi.fn(), setEditDraft: vi.fn(), setSearchOpen: vi.fn(), setSearchQuery: vi.fn(),
        setWorkspaceOverlay: vi.fn(), setChannelEditOpen: vi.fn(), setChannelCreateGroup: vi.fn(), setAvailableMembers: vi.fn(),
        setAvailableMembersLoaded: vi.fn(), setChannels, setMessages: vi.fn(), setAuthUser, setAuthState,
        setBackendState: vi.fn(), setMyPresence: vi.fn(), setTypingUsers: vi.fn(), setThreadRoot: vi.fn(),
        setThreadDraft: vi.fn(),
        setThreadReplies: vi.fn(), setActionError,
      })
      return null
    }
    renderToString(createElement(Harness))

    await expect(actions.logout()).resolves.toBe(false)

    expect(setAuthUser).not.toHaveBeenCalled()
    expect(setAuthState).not.toHaveBeenCalled()
    expect(setChannels).not.toHaveBeenCalled()
    expect(setActionError).toHaveBeenCalledWith(expect.any(String))
  })
})
