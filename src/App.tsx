import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { ChatPanel } from './components/ChatPanel'
import { ChannelCreateDialog } from './components/ChannelCreateDialog'
import { ChannelEditDialog } from './components/ChannelEditDialog'
import { DetailsPanel } from './components/DetailsPanel'
import { ThreadPanel } from './components/ThreadPanel'
import { WorkspaceSidebar } from './components/WorkspaceSidebar'
import { WorkspaceOverlay, type WorkspaceOverlayKind } from './components/WorkspaceOverlay'
import { chatApi, type ApiChannelMember, type ApiMember, type ApiMessage, type ApiUser } from './services/chatApi'
import { useChannelManagement } from './hooks/useChannelManagement'
import { useChatRealtime } from './hooks/useChatRealtime'
import { useSavedMessages } from './hooks/useSavedMessages'
import { useThread } from './hooks/useThread'
import { useWorkSummary } from './hooks/useWorkSummary'
import { demoUser, fromApiChannel, fromApiMessage, initialChannels, initialMessages, mergeMessage, type Channel, type Message } from './types/chat'
import { t } from './i18n'

type PaginationState = { nextCursor?: string; hasMore: boolean; loading: boolean }
type BackendState = 'checking' | 'ready' | 'unavailable'

function App() {
  const [channels, setChannels] = useState(initialChannels)
  const [selectedChannelId, setSelectedChannelId] = useState('design-system')
  const [messages, setMessages] = useState(initialMessages)
  const [draft, setDraft] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState('')
  const [showDetails, setShowDetails] = useState(true)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [backendState, setBackendState] = useState<BackendState>('checking')
  const [actionError, setActionError] = useState<string | null>(null)
  const [authState, setAuthState] = useState<'checking' | 'anonymous' | 'authenticated'>('checking')
  const [authUser, setAuthUser] = useState<ApiUser | null>(null)
  const [availableMembers, setAvailableMembers] = useState<ApiMember[]>([])
  const [availableMembersLoaded, setAvailableMembersLoaded] = useState(false)
  const [selectedChannelMembers, setSelectedChannelMembers] = useState<ApiChannelMember[]>([])
  const [selectedChannelMembersLoaded, setSelectedChannelMembersLoaded] = useState(false)
  const [myPresence, setMyPresence] = useState<NonNullable<Channel['presence']>>('online')
  const [messagePagination, setMessagePagination] = useState<Record<string, PaginationState>>({})
  const [typingUsers, setTypingUsers] = useState<Record<string, Record<string, string>>>({})
  const [highlightedMessageId, setHighlightedMessageId] = useState<string | null>(null)
  const [workspaceOverlay, setWorkspaceOverlay] = useState<WorkspaceOverlayKind | null>(null)
  const [channelCreateGroup, setChannelCreateGroup] = useState<string | null>(null)
  const [channelEditOpen, setChannelEditOpen] = useState(false)
  const [savedMessages, setSavedMessages] = useSavedMessages(authUser?.id ?? null)
  const messageListRef = useRef<HTMLDivElement>(null)
  const selectedChannelRef = useRef(selectedChannelId)
  const loadMessagesRef = useRef<(channelId: string) => Promise<void>>(async () => undefined)
  const refreshChannelsRef = useRef<(advanceCursor?: boolean) => Promise<void>>(async () => undefined)
  const refreshSelectedChannelMembersRef = useRef<() => Promise<void>>(async () => undefined)
  const advanceEventCursorRef = useRef<(cursor: number) => void>(() => undefined)
  const typingTimerRef = useRef<number | undefined>(undefined)
  const typingActiveRef = useRef(false)
  const messageElementsRef = useRef<Record<string, HTMLElement | null>>({})
  const channelRefreshSequenceRef = useRef(0)

  const backendReady = backendState === 'ready'
  const backendUnavailable = backendState === 'unavailable'
  const backendUnavailableMessage = t('errors.backendUnavailable')

  const currentUser = authUser ?? demoUser
  const selectedChannel = channels.find((channel) => channel.id === selectedChannelId) ?? channels[0] ?? initialChannels[0]
  const currentMessages = messages[selectedChannelId] ?? []
  const visibleMessages = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    if (!query) return currentMessages
    return currentMessages.filter((message) => `${message.author} ${message.body}`.toLowerCase().includes(query))
  }, [currentMessages, searchQuery])
  const channelGroups = ['Engineering', 'Product']
  const savedMessageIds = useMemo(() => new Set(savedMessages.filter((item) => item.channelId === selectedChannelId).map((item) => item.messageId)), [savedMessages, selectedChannelId])
  const unreadCount = useMemo(() => channels.reduce((total, channel) => total + channel.unread, 0), [channels])
  const threadCount = useMemo(() => Object.values(messages).flat().filter((message) => (message.threadCount ?? 0) > 0).length, [messages])
  const presenceFor = (handle: string, fallback: NonNullable<Channel['presence']>) => channels.find((channel) => channel.id === handle)?.presence ?? fallback
  const members = [
    { name: 'Ayaka Mori', handle: 'ayaka', initials: 'AM', role: t('details.roles.productDesigner'), presence: presenceFor('ayaka', 'online'), color: 'linear-gradient(135deg, #f8c291, #e55039)' },
    { name: 'Ken Ito', handle: 'ken', initials: 'KI', role: t('details.roles.frontendEngineer'), presence: presenceFor('ken', 'away'), color: 'linear-gradient(135deg, #82ccdd, #60a3bc)' },
    { name: currentUser.name, handle: currentUser.handle, initials: currentUser.initials, role: t('details.roles.productEngineer'), presence: myPresence, color: currentUser.color },
    { name: 'Mio Tanaka', handle: 'mio', initials: 'MT', role: t('details.roles.backendEngineer'), presence: 'offline' as const, color: 'linear-gradient(135deg, #b8e994, #78e08f)' },
  ]
  const conversationMembers = selectedChannel.kind === 'dm'
    ? [members.find((member) => member.handle === currentUser.handle) ?? members[2], { name: selectedChannel.name, handle: selectedChannel.id, initials: selectedChannel.initials ?? '?', role: selectedChannel.id === 'orbit-ai' ? t('details.roles.aiAssistant') : t('details.roles.member'), presence: selectedChannel.presence ?? 'offline', color: selectedChannel.color ?? '#394b6a' }]
    : members
  const detailMembers = backendReady && selectedChannelMembersLoaded
    ? selectedChannelMembers.map((member) => ({
      name: member.name,
      handle: member.handle,
      initials: member.initials,
      role: member.is_bot ? t('details.roles.aiAssistant') : member.role === 'owner' ? t('details.roles.owner') : t('details.roles.member'),
      presence: member.handle === currentUser.handle ? myPresence : presenceFor(member.handle, 'offline'),
      color: member.color,
    }))
    : backendReady ? [] : conversationMembers
  const currentChannelRole = selectedChannelMembers.find((member) => member.id === currentUser.id)?.role ?? 'member'
  const canEditSelectedChannel = selectedChannel.kind === 'channel' && (currentChannelRole === 'owner' || currentChannelRole === 'admin')
  const summary = useWorkSummary({ backendReady, backendUnavailableMessage, selectedChannelId, selectedChannelRef })
  const { workSummary, summaryLoading, summaryError, setSummaryError, generateSummary } = summary
  const thread = useThread({ backendReady, selectedChannelRef, messages, setActionError })
  const { threadRoot, setThreadRoot, threadReplies, setThreadReplies, threadDraft, setThreadDraft, threadLoading, threadRootRef, threadReplyIDsRef, openThread, closeThread, invalidateRequest } = thread

  const loadMessages = async (channelId: string, before?: string) => {
    const page = await chatApi.listMessages(channelId, before)
    advanceEventCursorRef.current(page.cursor)
    setMessages((current) => {
      const incoming = page.messages.filter((message) => !message.parent_message_id).map(fromApiMessage)
      if (!before) return { ...current, [channelId]: incoming }
      const existing = current[channelId] ?? []
      const existingIDs = new Set(existing.map((message) => message.id))
      return { ...current, [channelId]: [...incoming.filter((message) => !existingIDs.has(message.id)), ...existing] }
    })
    setMessagePagination((current) => ({ ...current, [channelId]: { nextCursor: page.next_cursor, hasMore: page.has_more, loading: false } }))
  }

  loadMessagesRef.current = (channelId) => loadMessages(channelId)
  const refreshChannels = async (advanceCursor = false) => {
    const requestSequence = channelRefreshSequenceRef.current + 1
    channelRefreshSequenceRef.current = requestSequence
    const remote = await chatApi.listChannels()
    if (requestSequence !== channelRefreshSequenceRef.current) return
    setChannels(remote.channels.map(fromApiChannel))
    setSelectedChannelId((current) => remote.channels.some((channel) => channel.id === current) ? current : remote.channels[0]?.id ?? current)
    if (advanceCursor) advanceEventCursorRef.current(remote.cursor)
  }
  refreshChannelsRef.current = refreshChannels
  const refreshSelectedChannelMembers = async (channelId = selectedChannelRef.current) => {
    if (!backendReady) return
    const response = await chatApi.listChannelMembers(channelId)
    if (selectedChannelRef.current !== channelId) return
    setSelectedChannelMembers(response.members)
    setSelectedChannelMembersLoaded(true)
  }
  refreshSelectedChannelMembersRef.current = () => refreshSelectedChannelMembers()
  const realtime = useChatRealtime({
    enabled: backendReady,
    currentUser,
    selectedChannelRef,
    threadRootRef,
    threadReplyIDsRef,
    loadMessagesRef,
    refreshChannelsRef,
    refreshSelectedChannelMembersRef,
    setChannels,
    setMessages,
    setTypingUsers,
    setMyPresence,
    setThreadRoot,
    setThreadReplies,
  })
  const { connection, send, addThreadReply } = realtime
  advanceEventCursorRef.current = realtime.advanceEventCursor
  const { openChannelCreate, createChannel, updateChannel } = useChannelManagement({
    backendReady,
    backendUnavailableMessage,
    selectedChannelId,
    setChannels,
    setMessages,
    setSelectedChannelId,
    setSelectedChannelMembers,
    setChannelCreateGroup,
    setChannelEditOpen,
    setActionError,
  })

  useEffect(() => {
    chatApi.me().then((user) => { setAuthUser(user); setAuthState('authenticated') }).catch(() => setAuthState('anonymous'))
  }, [])

  useEffect(() => { selectedChannelRef.current = selectedChannelId }, [selectedChannelId])
  useEffect(() => { threadRootRef.current = threadRoot }, [threadRoot])
  useEffect(() => { setHighlightedMessageId(null) }, [selectedChannelId])

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setWorkspaceOverlay('search')
      }
      if (event.key === 'Escape') setWorkspaceOverlay(null)
    }
    window.addEventListener('keydown', handleShortcut)
    return () => window.removeEventListener('keydown', handleShortcut)
  }, [])

  useEffect(() => {
    if (authState !== 'authenticated') return
    setBackendState('checking')
    setActionError(null)
    let disposed = false
    let loaded = false
    const loadChannels = async () => {
      if (disposed || loaded) return
      try {
        await refreshChannels(true)
        if (disposed) return
        setAvailableMembers([])
        setAvailableMembersLoaded(false)
        setBackendState('ready')
        loaded = true
        void chatApi.listUsers().then((memberResponse) => {
          if (!disposed) {
            setAvailableMembers(memberResponse.users)
            setAvailableMembersLoaded(true)
          }
        }).catch(() => {
          if (!disposed) setAvailableMembersLoaded(true)
        })
      } catch {
        if (!disposed) setBackendState('unavailable')
      }
    }
    void loadChannels()
    const retryTimer = window.setInterval(() => { void loadChannels() }, 1500)
    return () => { disposed = true; window.clearInterval(retryTimer) }
  }, [authState])

  useEffect(() => {
    if (!backendReady) return
    let disposed = false
    setSelectedChannelMembersLoaded(false)
    setSelectedChannelMembers([])
    void chatApi.listChannelMembers(selectedChannelId).then((response) => {
      if (!disposed) {
        setSelectedChannelMembers(response.members)
        setSelectedChannelMembersLoaded(true)
      }
    }).catch(() => {
      if (!disposed) {
        setSelectedChannelMembers([])
        setSelectedChannelMembersLoaded(true)
      }
    })
    return () => { disposed = true }
  }, [backendReady, selectedChannelId])

  useEffect(() => {
    if (!backendReady) return
    let disposed = false
    void loadMessages(selectedChannelId).then(() => {
      if (!disposed) void chatApi.markChannelRead(selectedChannelId)
    }).catch(() => undefined)
    return () => { disposed = true }
  }, [backendReady, selectedChannelId])

  useEffect(() => {
    const list = messageListRef.current
    if (list && !searchQuery) list.scrollTop = list.scrollHeight
  }, [selectedChannelId])

  const stopTyping = () => {
    if (!typingActiveRef.current) return
    typingActiveRef.current = false
    if (typingTimerRef.current) window.clearTimeout(typingTimerRef.current)
    typingTimerRef.current = undefined
    send({ type: 'typing.stopped', channel_id: selectedChannelRef.current })
  }

  const onDraftChange = (value: string) => {
    setDraft(value)
    if (!backendReady || !value.trim()) {
      stopTyping()
      return
    }
    if (!typingActiveRef.current) {
      typingActiveRef.current = true
      send({ type: 'typing.started', channel_id: selectedChannelId })
    }
    if (typingTimerRef.current) window.clearTimeout(typingTimerRef.current)
    typingTimerRef.current = window.setTimeout(stopTyping, 1500)
  }

  const selectChannel = (channel: Channel) => {
    stopTyping()
    invalidateRequest()
    selectedChannelRef.current = channel.id
    setSelectedChannelId(channel.id)
    threadRootRef.current = null
    setThreadRoot(null)
    setThreadReplies([])
    threadReplyIDsRef.current.clear()
    setSearchQuery('')
    setChannelEditOpen(false)
    if (backendReady) {
      setChannels((current) => current.map((item) => item.id === channel.id ? { ...item, unread: 0 } : item))
      void chatApi.markChannelRead(channel.id).catch(() => setActionError(t('errors.readState')))
    }
  }

  const jumpToMessage = (messageId: string) => {
    const element = messageElementsRef.current[messageId]
    if (!element) {
      setSummaryError(t('errors.sourceMissing'))
      return
    }
    element.scrollIntoView({ behavior: 'smooth', block: 'center' })
    setHighlightedMessageId(messageId)
    window.setTimeout(() => setHighlightedMessageId((current) => current === messageId ? null : current), 1800)
  }

  const loadOlderMessages = () => {
    const pagination = messagePagination[selectedChannelId]
    if (!backendReady || !pagination?.hasMore || !pagination.nextCursor || pagination.loading) return
    setMessagePagination((current) => ({ ...current, [selectedChannelId]: { ...pagination, loading: true } }))
    void loadMessages(selectedChannelId, pagination.nextCursor).catch(() => setMessagePagination((current) => ({ ...current, [selectedChannelId]: { ...pagination, loading: false } })))
  }

  const sendThreadReply = async () => {
    const body = threadDraft.trim()
    if (!body || !threadRoot) return
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    setThreadDraft('')
    try {
      addThreadReply(fromApiMessage(await chatApi.createMessage(selectedChannelId, { body, parent_message_id: threadRoot.id })), selectedChannelId, true)
    } catch {
      setThreadDraft(body)
      setActionError(t('errors.replySend'))
    }
  }

  const upsertMessage = (remoteMessage: ApiMessage) => {
    const incoming = fromApiMessage(remoteMessage)
    setMessages((current) => {
      const existing = current[remoteMessage.channel_id] ?? []
      const index = existing.findIndex((message) => message.id === incoming.id)
      const next = [...existing]
      if (index >= 0) next[index] = mergeMessage(existing[index], incoming)
      else next.push(incoming)
      return { ...current, [remoteMessage.channel_id]: next }
    })
  }

  const sendMessage = async () => {
    const body = draft.trim()
    if (!body) return
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    stopTyping()
    setDraft('')
    try {
      upsertMessage(await chatApi.createMessage(selectedChannelId, { body }))
      setActionError(null)
    } catch {
      setDraft(body)
      setActionError(t('errors.messageSend'))
      return
    }
    window.requestAnimationFrame(() => { const list = messageListRef.current; if (list) list.scrollTop = list.scrollHeight })
  }

  const updateMessage = async () => {
    const body = editDraft.trim()
    if (!editingId || !body) return
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    try {
      upsertMessage(await chatApi.updateMessage(editingId, body))
      setActionError(null)
    } catch {
      setActionError(t('errors.messageEdit'))
      return
    }
    setEditingId(null)
    setEditDraft('')
  }

  const deleteMessage = async (messageId: string) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    try {
      await chatApi.deleteMessage(messageId)
      await loadMessages(selectedChannelId)
      setThreadReplies((current) => current.filter((message) => message.id !== messageId))
      setActionError(null)
    } catch {
      setActionError(t('errors.messageDelete'))
    }
  }

  const toggleReaction = async (messageId: string, emoji: string) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    const message = (messages[selectedChannelId] ?? []).find((item) => item.id === messageId)
    const existing = message?.reactions?.find((reaction) => reaction.emoji === emoji)
    try {
      upsertMessage(await (existing?.reacted ? chatApi.removeReaction(messageId, emoji) : chatApi.addReaction(messageId, emoji)))
    } catch {
      setActionError(t('errors.reactionUpdate'))
    }
  }

  const toggleSaved = (messageId: string) => {
    setSavedMessages((current) => {
      const exists = current.some((item) => item.channelId === selectedChannelId && item.messageId === messageId)
      if (exists) return current.filter((item) => !(item.channelId === selectedChannelId && item.messageId === messageId))
      return [...current, { channelId: selectedChannelId, messageId }]
    })
  }

  const startEditing = (message: Message) => { setEditingId(message.id); setEditDraft(message.body); setDraft('') }
  const onComposerKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); if (editingId) void updateMessage(); else void sendMessage() } }
  const onThreadKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void sendThreadReply() } }
  const changePresence = (nextPresence: NonNullable<Channel['presence']>) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    setMyPresence(nextPresence)
    send({ type: 'presence.changed', presence: nextPresence })
    setActionError(null)
  }
  const updateProfile = async (name: string) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      throw new Error('backend unavailable')
    }
    try {
      const updatedUser = await chatApi.updateProfile(name)
      setAuthUser(updatedUser)
      setMessages((current) => Object.fromEntries(Object.entries(current).map(([channelId, channelMessages]) => [channelId, channelMessages.map((message) => message.authorID === currentUser.id ? { ...message, author: updatedUser.name, initials: updatedUser.initials } : message)])))
      setThreadRoot((current) => current?.authorID === currentUser.id ? { ...current, author: updatedUser.name, initials: updatedUser.initials } : current)
      setThreadReplies((current) => current.map((message) => message.authorID === currentUser.id ? { ...message, author: updatedUser.name, initials: updatedUser.initials } : message))
      setActionError(null)
    } catch (error) {
      setActionError(t('errors.profileUpdate'))
      throw error
    }
  }
  const typingLabel = Object.values(typingUsers[selectedChannelId] ?? {}).join('、')
  const logout = async () => { try { await chatApi.logout() } finally { setAuthUser(null); setAuthState('anonymous'); setBackendState('checking') } }
  const openThreadFromOverlay = (channel: Channel, message: Message) => {
    selectChannel(channel)
    void openThread(message)
  }

  if (authState !== 'authenticated' || !authUser) return <AuthScreen onAuthenticated={(user) => { setAuthUser(user); setAuthState('authenticated') }} />

  return (
    <div className="app-shell">
      <WorkspaceSidebar channels={channels} selectedChannelId={selectedChannelId} currentUser={currentUser} myPresence={myPresence} channelGroups={channelGroups} onSelectChannel={selectChannel} onAddChannel={openChannelCreate} onChangePresence={changePresence} onUpdateProfile={updateProfile} onLogout={() => void logout()} unreadCount={unreadCount} savedCount={savedMessages.length} threadCount={threadCount} onOpenSearch={() => setWorkspaceOverlay('search')} onOpenQuickLink={setWorkspaceOverlay} onOpenWorkspace={() => setWorkspaceOverlay('workspace')} onOpenHelp={() => setWorkspaceOverlay('help')} />
      {channelCreateGroup && <ChannelCreateDialog initialGroup={channelCreateGroup} groups={channelGroups} members={availableMembers} currentUserId={currentUser.id} onCreate={createChannel} onClose={() => setChannelCreateGroup(null)} />}
      {channelEditOpen && <ChannelEditDialog channel={selectedChannel} members={availableMembers} channelMembers={selectedChannelMembers} currentUserId={currentUser.id} currentUserRole={currentChannelRole} onSave={updateChannel} onClose={() => setChannelEditOpen(false)} />}
      <ChatPanel selectedChannel={selectedChannel} visibleMessages={visibleMessages} currentUser={currentUser} backendAvailable={backendReady} errorMessage={actionError ?? (backendUnavailable ? backendUnavailableMessage : undefined)} searchOpen={searchOpen} searchQuery={searchQuery} showDetails={showDetails} editingId={editingId} draft={draft} editDraft={editDraft} messageListRef={messageListRef} messageElementsRef={messageElementsRef} highlightedMessageId={highlightedMessageId} hasMore={messagePagination[selectedChannelId]?.hasMore ?? false} loadingOlder={messagePagination[selectedChannelId]?.loading ?? false} onLoadOlder={loadOlderMessages} onSearchOpenChange={setSearchOpen} onSearchQueryChange={setSearchQuery} onToggleDetails={() => setShowDetails((open) => !open)} canEditChannel={canEditSelectedChannel} onOpenChannelEdit={() => setChannelEditOpen(true)} onToggleReaction={toggleReaction} savedMessageIds={savedMessageIds} onToggleSaved={toggleSaved} onOpenThread={(message) => void openThread(message)} typingLabel={typingLabel} onStartEditing={startEditing} onDeleteMessage={(messageId) => void deleteMessage(messageId)} onDraftChange={onDraftChange} onEditDraftChange={setEditDraft} onComposerKeyDown={onComposerKeyDown} onSubmit={() => { if (editingId) void updateMessage(); else void sendMessage() }} onCancelEditing={() => { setEditingId(null); setEditDraft('') }} />
      {workspaceOverlay && <WorkspaceOverlay kind={workspaceOverlay} channels={channels} messages={messages} savedMessages={savedMessages} memberCount={availableMembersLoaded ? availableMembers.length : undefined} connection={connection} onSelectChannel={selectChannel} onOpenThread={openThreadFromOverlay} onClose={() => setWorkspaceOverlay(null)} />}
      {showDetails && <DetailsPanel selectedChannel={selectedChannel} members={detailMembers} summary={workSummary} summaryLoading={summaryLoading} summaryError={summaryError ?? undefined} onGenerateSummary={() => void generateSummary()} onJumpToMessage={jumpToMessage} onClose={() => setShowDetails(false)} />}
      {threadRoot && <ThreadPanel root={threadRoot} replies={threadReplies} draft={threadDraft} loading={threadLoading} onDraftChange={setThreadDraft} onKeyDown={onThreadKeyDown} onSubmit={() => void sendThreadReply()} onClose={closeThread} />}
    </div>
  )
}

export default App
