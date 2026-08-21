import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { ChatPanel } from './components/ChatPanel'
import { DetailsPanel } from './components/DetailsPanel'
import { ThreadPanel } from './components/ThreadPanel'
import { WorkspaceSidebar } from './components/WorkspaceSidebar'
import { WorkspaceOverlay, type SavedMessageRef, type WorkspaceOverlayKind } from './components/WorkspaceOverlay'
import { chatApi, type ApiChannelSummary, type ApiMessage, type ApiUser } from './services/chatApi'
import { useChatRealtime } from './hooks/useChatRealtime'
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
  const [myPresence, setMyPresence] = useState<NonNullable<Channel['presence']>>('online')
  const [messagePagination, setMessagePagination] = useState<Record<string, PaginationState>>({})
  const [typingUsers, setTypingUsers] = useState<Record<string, Record<string, string>>>({})
  const [threadRoot, setThreadRoot] = useState<Message | null>(null)
  const [threadReplies, setThreadReplies] = useState<Message[]>([])
  const [threadDraft, setThreadDraft] = useState('')
  const [threadLoading, setThreadLoading] = useState(false)
  const [workSummary, setWorkSummary] = useState<ApiChannelSummary | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [summaryError, setSummaryError] = useState<string | null>(null)
  const [highlightedMessageId, setHighlightedMessageId] = useState<string | null>(null)
  const [workspaceOverlay, setWorkspaceOverlay] = useState<WorkspaceOverlayKind | null>(null)
  const [savedMessages, setSavedMessages] = useState<SavedMessageRef[]>(() => {
    try {
      const stored = window.localStorage.getItem('orbit:saved-message-refs')
      return stored ? JSON.parse(stored) as SavedMessageRef[] : []
    } catch {
      return []
    }
  })
  const messageListRef = useRef<HTMLDivElement>(null)
  const selectedChannelRef = useRef(selectedChannelId)
  const loadMessagesRef = useRef<(channelId: string) => Promise<void>>(async () => undefined)
  const advanceEventCursorRef = useRef<(cursor: number) => void>(() => undefined)
  const typingTimerRef = useRef<number | undefined>(undefined)
  const typingActiveRef = useRef(false)
  const threadReplyIDsRef = useRef<Set<string>>(new Set())
  const threadRootRef = useRef<Message | null>(null)
  const messageElementsRef = useRef<Record<string, HTMLElement | null>>({})
  const summaryRequestSequenceRef = useRef(0)
  const summaryAbortControllerRef = useRef<AbortController | null>(null)

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
  const realtime = useChatRealtime({
    enabled: backendReady,
    currentUser,
    selectedChannelRef,
    threadRootRef,
    threadReplyIDsRef,
    loadMessagesRef,
    setChannels,
    setMessages,
    setTypingUsers,
    setMyPresence,
    setThreadRoot,
    setThreadReplies,
  })
  const { connection, send, addThreadReply } = realtime
  advanceEventCursorRef.current = realtime.advanceEventCursor

  useEffect(() => {
    chatApi.me().then((user) => { setAuthUser(user); setAuthState('authenticated') }).catch(() => setAuthState('anonymous'))
  }, [])

  useEffect(() => { selectedChannelRef.current = selectedChannelId }, [selectedChannelId])
  useEffect(() => { threadRootRef.current = threadRoot }, [threadRoot])
  useEffect(() => {
    summaryRequestSequenceRef.current += 1
    summaryAbortControllerRef.current?.abort()
    summaryAbortControllerRef.current = null
    setSummaryLoading(false)
    setWorkSummary(null)
    setSummaryError(null)
    setHighlightedMessageId(null)
  }, [selectedChannelId])

  useEffect(() => () => {
    summaryRequestSequenceRef.current += 1
    summaryAbortControllerRef.current?.abort()
    summaryAbortControllerRef.current = null
  }, [])

  useEffect(() => {
    window.localStorage.setItem('orbit:saved-message-refs', JSON.stringify(savedMessages))
  }, [savedMessages])

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
        const remote = await chatApi.listChannels()
        if (disposed) return
        setChannels(remote.channels.map(fromApiChannel))
        advanceEventCursorRef.current(remote.cursor)
        setBackendState('ready')
        loaded = true
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
    selectedChannelRef.current = channel.id
    setSelectedChannelId(channel.id)
    threadRootRef.current = null
    setThreadRoot(null)
    setThreadReplies([])
    threadReplyIDsRef.current.clear()
    setSearchQuery('')
    if (backendReady) {
      setChannels((current) => current.map((item) => item.id === channel.id ? { ...item, unread: 0 } : item))
      void chatApi.markChannelRead(channel.id).catch(() => setActionError(t('errors.readState')))
    }
  }

  const generateSummary = async () => {
    if (!backendReady) {
      setSummaryError(backendUnavailableMessage)
      return
    }
    const requestChannelId = selectedChannelId
    const requestSequence = summaryRequestSequenceRef.current + 1
    summaryRequestSequenceRef.current = requestSequence
    summaryAbortControllerRef.current?.abort()
    const controller = new AbortController()
    summaryAbortControllerRef.current = controller
    setSummaryLoading(true)
    setSummaryError(null)
    try {
      const summary = await chatApi.summarizeChannel(requestChannelId, controller.signal)
      if (controller.signal.aborted || requestSequence !== summaryRequestSequenceRef.current || selectedChannelRef.current !== requestChannelId) return
      setWorkSummary(summary)
    } catch {
      if (controller.signal.aborted || requestSequence !== summaryRequestSequenceRef.current || selectedChannelRef.current !== requestChannelId) return
      setSummaryError(t('errors.summary'))
    } finally {
      if (requestSequence === summaryRequestSequenceRef.current) {
        summaryAbortControllerRef.current = null
        setSummaryLoading(false)
      }
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

  const openThread = async (message: Message) => {
    threadRootRef.current = message
    setThreadRoot(message)
    setThreadDraft('')
    setThreadLoading(true)
    try {
      if (backendReady) {
        const page = await chatApi.listThreadMessages(message.id)
        const loadedReplies = page.messages.map(fromApiMessage)
        threadReplyIDsRef.current = new Set(loadedReplies.map((reply) => reply.id))
        setThreadReplies(loadedReplies)
      } else {
        const loadedReplies = (messages[selectedChannelId] ?? []).filter((item) => item.parentMessageId === message.id)
        threadReplyIDsRef.current = new Set(loadedReplies.map((reply) => reply.id))
        setThreadReplies(loadedReplies)
      }
    } finally {
      setThreadLoading(false)
    }
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

  const addChannel = async () => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    const name = `new-room-${channels.filter((channel) => channel.kind === 'channel').length + 1}`
    try {
      const channel = fromApiChannel(await chatApi.createChannel({ name, group: 'Product', kind: 'channel', description: t('channel.newDescription') }))
      setChannels((current) => [...current, channel])
      setMessages((current) => ({ ...current, [channel.id]: [] }))
      setSelectedChannelId(channel.id)
    } catch {
      setActionError(t('errors.channelCreate'))
    }
  }

  const startEditing = (message: Message) => { setEditingId(message.id); setEditDraft(message.body); setDraft('') }
  const onComposerKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); if (editingId) void updateMessage(); else void sendMessage() } }
  const onThreadKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void sendThreadReply() } }
  const togglePresence = () => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    const nextPresence = myPresence === 'online' ? 'away' : 'online'
    setMyPresence(nextPresence)
    send({ type: 'presence.changed', presence: nextPresence })
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
      <WorkspaceSidebar channels={channels} selectedChannelId={selectedChannelId} currentUser={currentUser} myPresence={myPresence} channelGroups={channelGroups} onSelectChannel={selectChannel} onAddChannel={() => void addChannel()} onTogglePresence={togglePresence} onLogout={() => void logout()} unreadCount={unreadCount} savedCount={savedMessages.length} threadCount={threadCount} onOpenSearch={() => setWorkspaceOverlay('search')} onOpenQuickLink={setWorkspaceOverlay} onOpenWorkspace={() => setWorkspaceOverlay('workspace')} onOpenHelp={() => setWorkspaceOverlay('help')} onHome={() => { const home = channels.find((channel) => channel.id === 'general') ?? channels[0]; if (home) selectChannel(home) }} />
      <ChatPanel selectedChannel={selectedChannel} visibleMessages={visibleMessages} currentUser={currentUser} connection={connection} backendAvailable={backendReady} errorMessage={actionError ?? (backendUnavailable ? backendUnavailableMessage : undefined)} searchOpen={searchOpen} searchQuery={searchQuery} showDetails={showDetails} editingId={editingId} draft={draft} editDraft={editDraft} messageListRef={messageListRef} messageElementsRef={messageElementsRef} highlightedMessageId={highlightedMessageId} hasMore={messagePagination[selectedChannelId]?.hasMore ?? false} loadingOlder={messagePagination[selectedChannelId]?.loading ?? false} onLoadOlder={loadOlderMessages} onSearchOpenChange={setSearchOpen} onSearchQueryChange={setSearchQuery} onToggleDetails={() => setShowDetails((open) => !open)} onToggleReaction={toggleReaction} savedMessageIds={savedMessageIds} onToggleSaved={toggleSaved} onOpenThread={(message) => void openThread(message)} typingLabel={typingLabel} onStartEditing={startEditing} onDeleteMessage={(messageId) => void deleteMessage(messageId)} onDraftChange={onDraftChange} onEditDraftChange={setEditDraft} onComposerKeyDown={onComposerKeyDown} onSubmit={() => { if (editingId) void updateMessage(); else void sendMessage() }} onCancelEditing={() => { setEditingId(null); setEditDraft('') }} />
      {workspaceOverlay && <WorkspaceOverlay kind={workspaceOverlay} channels={channels} messages={messages} savedMessages={savedMessages} onSelectChannel={selectChannel} onOpenThread={openThreadFromOverlay} onClose={() => setWorkspaceOverlay(null)} />}
      {showDetails && <DetailsPanel selectedChannel={selectedChannel} members={conversationMembers} summary={workSummary} summaryLoading={summaryLoading} summaryError={summaryError ?? undefined} onGenerateSummary={() => void generateSummary()} onJumpToMessage={jumpToMessage} onClose={() => setShowDetails(false)} />}
      {threadRoot && <ThreadPanel root={threadRoot} replies={threadReplies} draft={threadDraft} loading={threadLoading} onDraftChange={setThreadDraft} onKeyDown={onThreadKeyDown} onSubmit={() => void sendThreadReply()} onClose={() => { threadRootRef.current = null; setThreadRoot(null); setThreadReplies([]); threadReplyIDsRef.current.clear() }} />}
    </div>
  )
}

export default App
