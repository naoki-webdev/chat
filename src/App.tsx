import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type SetStateAction } from 'react'
import { AuthScreen } from './components/AuthScreen'
import { ChatPanel } from './components/ChatPanel'
import { ChannelCreateDialog } from './components/ChannelCreateDialog'
import { ChannelEditDialog } from './components/ChannelEditDialog'
import { DetailsPanel } from './components/DetailsPanel'
import { ThreadPanel } from './components/ThreadPanel'
import { WorkspaceSidebar } from './components/WorkspaceSidebar'
import { WorkspaceOverlay } from './components/WorkspaceOverlay'
import { ChatApiError, chatApi, type ApiMember, type ApiUser } from './services/chatApi'
import { useChannelManagement } from './hooks/useChannelManagement'
import { useChannelMembers } from './hooks/useChannelMembers'
import { useChatRealtime } from './hooks/useChatRealtime'
import { useChatMessages } from './hooks/useChatMessages'
import { useSavedMessages } from './hooks/useSavedMessages'
import { useThread } from './hooks/useThread'
import { useWorkSummary } from './hooks/useWorkSummary'
import { useWorkspaceOverlays } from './hooks/useWorkspaceOverlays'
import { useWorkspaceActions } from './hooks/useWorkspaceActions'
import { enqueueRealtimeTask } from './hooks/realtimeQueue'
import { fromApiChannel, fromApiMessage, type Channel, type Message } from './types/chat'
import { demoUser, initialChannels, initialMessages } from './types/demoData'
import { t } from './i18n'

type BackendState = 'checking' | 'ready' | 'unavailable'

function App() {
  const [channels, setChannels] = useState(initialChannels)
  const [channelGroups, setChannelGroups] = useState(['Engineering', 'Product'])
  const [selectedChannelId, setSelectedChannelId] = useState('design-system')
  const [messages, setMessages] = useState(initialMessages)
  const [showDetails, setShowDetails] = useState(true)
  const [searchOpen, setSearchOpen] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [backendState, setBackendState] = useState<BackendState>('checking')
  const [actionError, setActionError] = useState<string | null>(null)
  const [authState, setAuthState] = useState<'checking' | 'anonymous' | 'unavailable' | 'authenticated'>('checking')
  const [authUser, setAuthUser] = useState<ApiUser | null>(null)
  const authUserRef = useRef<ApiUser | null>(null)
  const sessionVersionRef = useRef(0)
  const setAuthUserWithRef = useCallback((action: SetStateAction<ApiUser | null>) => {
    const nextUser = typeof action === 'function' ? action(authUserRef.current) : action
    if (authUserRef.current?.id !== nextUser?.id || (authUserRef.current !== null && nextUser === null)) sessionVersionRef.current += 1
    authUserRef.current = nextUser
    setAuthUser(nextUser)
  }, [])
  const isSessionVersionActive = useCallback((version: number) => sessionVersionRef.current === version, [])
  const [availableMembers, setAvailableMembers] = useState<ApiMember[]>([])
  const [availableMembersLoaded, setAvailableMembersLoaded] = useState(false)
  const [myPresence, setMyPresence] = useState<NonNullable<Channel['presence']>>('online')
  const [typingUsers, setTypingUsers] = useState<Record<string, Record<string, string>>>({})
  const [highlightedMessageId, setHighlightedMessageId] = useState<string | null>(null)
  const [pendingSavedJump, setPendingSavedJump] = useState<{ channelId: string; messageId: string } | null>(null)
  const [channelCreateGroup, setChannelCreateGroup] = useState<string | null>(null)
  const [channelEditOpen, setChannelEditOpen] = useState(false)
  const [savedMessages, setSavedMessages] = useSavedMessages(authUser?.id ?? null)
  const selectedChannelRef = useRef(selectedChannelId)
  const refreshChannelsRef = useRef<(advanceCursor?: boolean) => Promise<void>>(async () => undefined)
  const refreshSelectedChannelMembersRef = useRef<() => Promise<void>>(async () => undefined)
  const advanceEventCursorRef = useRef<(cursor: number) => void>(() => undefined)
  const sendRealtimeRef = useRef<(payload: unknown) => void>(() => undefined)
  const channelRefreshSequenceRef = useRef(0)
  const realtimeQueueRef = useRef<Promise<void>>(Promise.resolve())
  const invalidateChannelRefresh = useCallback(() => {
    channelRefreshSequenceRef.current += 1
  }, [])

  const backendReady = backendState === 'ready'
  const backendUnavailable = backendState === 'unavailable'
  const backendUnavailableMessage = t('errors.backendUnavailable')

  const { members: selectedChannelMembers, loaded: selectedChannelMembersLoaded, refresh: refreshSelectedChannelMembers } = useChannelMembers({ backendReady, selectedChannelId, selectedChannelRef })

  const currentUser = authUser ?? demoUser
  const selectedChannel = channels.find((channel) => channel.id === selectedChannelId) ?? channels[0] ?? initialChannels[0]
  const { currentMessages, visibleMessages, workspaceOverlay, setWorkspaceOverlay, workspaceThreadItems, workspaceThreadCount, workspaceThreadsLoaded } = useWorkspaceOverlays({ backendReady, selectedChannelId, selectedChannelRef, messages, searchQuery })
  const savedMessageIds = useMemo(() => new Set(savedMessages.filter((item) => item.channelId === selectedChannelId).map((item) => item.messageId)), [savedMessages, selectedChannelId])
  const unreadCount = useMemo(() => channels.reduce((total, channel) => total + channel.unread, 0), [channels])
  const localThreadCount = useMemo(() => Object.values(messages).flat().filter((message) => (message.threadCount ?? 0) > 0).length, [messages])
  const threadCount = workspaceThreadsLoaded ? workspaceThreadCount : localThreadCount
  const presenceFor = (handle: string, fallback: NonNullable<Channel['presence']>, userID?: string) => channels.find((channel) => channel.peerUserID === userID || channel.id === handle)?.presence ?? fallback
  const members = [
    { id: 'u-ayaka', name: 'Ayaka Mori', handle: 'ayaka', initials: 'AM', role: t('details.roles.productDesigner'), presence: presenceFor('ayaka', 'online', 'u-ayaka'), color: 'linear-gradient(135deg, #f8c291, #e55039)' },
    { id: 'u-ken', name: 'Ken Ito', handle: 'ken', initials: 'KI', role: t('details.roles.frontendEngineer'), presence: presenceFor('ken', 'away', 'u-ken'), color: 'linear-gradient(135deg, #82ccdd, #60a3bc)' },
    { id: currentUser.id, name: currentUser.name, handle: currentUser.handle, initials: currentUser.initials, role: t('details.roles.productEngineer'), presence: myPresence, color: currentUser.color },
    { id: 'u-mio', name: 'Mio Tanaka', handle: 'mio', initials: 'MT', role: t('details.roles.backendEngineer'), presence: 'offline' as const, color: 'linear-gradient(135deg, #b8e994, #78e08f)' },
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
      presence: member.id === currentUser.id ? myPresence : presenceFor(member.handle, 'offline', member.id),
      color: member.color,
    }))
    : backendReady ? [] : conversationMembers
  const currentChannelRole = selectedChannelMembers.find((member) => member.id === currentUser.id)?.role ?? 'member'
  const canEditSelectedChannel = selectedChannel.kind === 'channel' && (currentChannelRole === 'owner' || currentChannelRole === 'admin')
  const summary = useWorkSummary({ backendReady, backendUnavailableMessage, selectedChannelId, selectedChannelRef })
  const { workSummary, summaryLoading, summaryError, setSummaryError, generateSummary } = summary
  const thread = useThread({ backendReady, selectedChannelRef, messages, setActionError, realtimeQueueRef })
  const { threadRoot, setThreadRoot, threadReplies, setThreadReplies, threadDraft, setThreadDraft, threadLoading, threadPagination, threadRootRef, threadReplyIDsRef, threadReplyElementsRef, openThread, loadOlderThreadReplies, closeThread, invalidateRequest } = thread
  const chatMessages = useChatMessages({
    backendReady,
    backendUnavailableMessage,
    selectedChannelId,
    selectedChannelRef,
    messages,
    setMessages,
    setActionError,
    setThreadReplies,
    advanceEventCursorRef,
    sendRealtime: (payload) => sendRealtimeRef.current(payload),
    realtimeQueueRef,
  })
  const {
    draft,
    setDraft,
    outgoingMessages,
    editingId,
    setEditingId,
    editDraft,
    setEditDraft,
    messagePagination,
    messageListRef,
    messageElementsRef,
    loadMessages,
    loadMessagesDirectRef,
    invalidateMessageRequests,
    loadOlderMessages,
    loadMessageUntil,
    stopTyping,
    onDraftChange,
    sendMessage,
    retryOutgoingMessage,
    updateMessage,
    deleteMessage,
    toggleReaction,
    startEditing,
    onComposerKeyDown,
  } = chatMessages
  const loadMessagesRef = useRef(loadMessages)
  loadMessagesRef.current = loadMessages

  const refreshChannels = async (advanceCursor = false) => {
    const requestSequence = channelRefreshSequenceRef.current + 1
    channelRefreshSequenceRef.current = requestSequence
    const remote = await chatApi.listChannels()
    if (requestSequence !== channelRefreshSequenceRef.current) return
    setChannels(remote.channels.map(fromApiChannel))
    if (remote.groups?.length) setChannelGroups(remote.groups)
    setSelectedChannelId((current) => remote.channels.some((channel) => channel.id === current) ? current : remote.channels[0]?.id ?? current)
    if (advanceCursor) advanceEventCursorRef.current(remote.cursor)
  }
  refreshChannelsRef.current = refreshChannels
  refreshSelectedChannelMembersRef.current = () => refreshSelectedChannelMembers()
  const realtime = useChatRealtime({
    enabled: backendReady,
    currentUser,
    selectedChannelRef,
    threadRootRef,
    threadReplyIDsRef,
    loadMessagesDirectRef,
    realtimeQueueRef,
    refreshChannelsRef,
    refreshSelectedChannelMembersRef,
    onReadStateError: () => setActionError(t('errors.readState')),
    setChannels,
    setAuthUser: setAuthUserWithRef,
    setMessages,
    setTypingUsers,
    setMyPresence,
    setThreadRoot,
    setThreadReplies,
  })
  const { connection, send, addThreadReply, clearScheduledChannelReads } = realtime
  sendRealtimeRef.current = send
  advanceEventCursorRef.current = realtime.advanceEventCursor
  const workspaceActions = useWorkspaceActions({
    backendReady,
    backendUnavailableMessage,
    currentUser,
    selectedChannelRef,
    threadRootRef,
    threadReplyIDsRef,
    invalidateMessageRequests,
    invalidateChannelRefresh,
    clearScheduledChannelReads,
    stopTyping,
    invalidateThreadRequest: invalidateRequest,
    sendPresence: (presence) => send({ type: 'presence.changed', presence }),
    setSelectedChannelId,
    setChannelGroups,
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
    setAuthUser: setAuthUserWithRef,
    setAuthState,
    setBackendState,
    setMyPresence,
    setTypingUsers,
    setThreadDraft,
    setThreadRoot,
    setThreadReplies,
    setActionError,
  })
  const { selectChannel, changePresence, updateProfile, logout } = workspaceActions
  const selectChannelAndResetJump = useCallback((channel: Channel) => {
    setPendingSavedJump(null)
    selectChannel(channel)
  }, [selectChannel])
  const { openChannelCreate, createChannel, updateChannel } = useChannelManagement({
    backendReady,
    backendUnavailableMessage,
    selectedChannelId,
    sessionVersion: sessionVersionRef.current,
    isSessionVersionActive,
    selectChannel,
    setChannels,
    setMessages,
    refreshSelectedChannelMembers,
    setChannelCreateGroup,
    setChannelEditOpen,
    setActionError,
  })

  useEffect(() => {
    chatApi.me().then((user) => { setAuthUserWithRef(user); setAuthState('authenticated') }).catch((error) => {
      setAuthState(error instanceof ChatApiError && error.status === 401 ? 'anonymous' : 'unavailable')
    })
  }, [setAuthUserWithRef])

  useEffect(() => { selectedChannelRef.current = selectedChannelId }, [selectedChannelId])
  useEffect(() => { threadRootRef.current = threadRoot }, [threadRoot, threadRootRef])
  useEffect(() => { setHighlightedMessageId(null) }, [selectedChannelId])

  useEffect(() => {
    if (!pendingSavedJump || pendingSavedJump.channelId !== selectedChannelId) return
    const element = messageElementsRef.current[pendingSavedJump.messageId]
    if (!element) return
    window.requestAnimationFrame(() => {
      element.scrollIntoView({ behavior: 'smooth', block: 'center' })
      setHighlightedMessageId(pendingSavedJump.messageId)
      window.setTimeout(() => setHighlightedMessageId((current) => current === pendingSavedJump.messageId ? null : current), 1800)
      setPendingSavedJump((current) => current?.messageId === pendingSavedJump.messageId ? null : current)
    })
  }, [messageElementsRef, messages, pendingSavedJump, selectedChannelId])

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
  }, [setWorkspaceOverlay])

  useEffect(() => {
    if (authState !== 'authenticated') return
    setBackendState('checking')
    setActionError(null)
    let disposed = false
    let loaded = false
    let requestInProgress = false
    let retryTimer: number | undefined
    const scheduleRetry = () => {
      if (disposed || loaded || retryTimer !== undefined) return
      retryTimer = window.setTimeout(() => {
        retryTimer = undefined
        void loadChannels()
      }, 1500)
    }
    const loadChannels = async () => {
      if (disposed || loaded || requestInProgress) return
      requestInProgress = true
      try {
        const expectedRefreshSequence = channelRefreshSequenceRef.current + 1
        await refreshChannels(true)
        if (disposed) return
        if (expectedRefreshSequence !== channelRefreshSequenceRef.current) {
          scheduleRetry()
          return
        }
        setAvailableMembers([])
        setAvailableMembersLoaded(false)
        setBackendState('ready')
        loaded = true
        const membersRequestSequence = channelRefreshSequenceRef.current
        void chatApi.listUsers().then((memberResponse) => {
          if (!disposed && membersRequestSequence === channelRefreshSequenceRef.current) {
            setAvailableMembers(memberResponse.users)
            setAvailableMembersLoaded(true)
          }
        }).catch(() => {
          if (!disposed && membersRequestSequence === channelRefreshSequenceRef.current) setAvailableMembersLoaded(true)
        })
      } catch {
        if (!disposed) {
          setBackendState('unavailable')
          scheduleRetry()
        }
      } finally {
        requestInProgress = false
      }
    }
    void loadChannels()
    return () => {
      disposed = true
      if (retryTimer !== undefined) window.clearTimeout(retryTimer)
    }
  }, [authState])

  useEffect(() => {
    if (!backendReady) return
    let disposed = false
    void loadMessagesRef.current(selectedChannelId).then(() => {
      if (!disposed) void chatApi.markChannelRead(selectedChannelId).catch(() => setActionError(t('errors.readState')))
    }).catch(() => undefined)
    return () => { disposed = true }
  }, [backendReady, selectedChannelId])

  useEffect(() => {
    const list = messageListRef.current
    if (list && !searchQuery) list.scrollTop = list.scrollHeight
    // チャンネル変更では初期スクロール位置を制御しますが、検索変更では制御しません。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedChannelId])

  const jumpToMessage = (messageId: string, parentMessageId?: string) => {
    const element = messageElementsRef.current[messageId]
    if (element) {
      element.scrollIntoView({ behavior: 'smooth', block: 'center' })
      setHighlightedMessageId(messageId)
      window.setTimeout(() => setHighlightedMessageId((current) => current === messageId ? null : current), 1800)
      return
    }
    const threadElement = threadReplyElementsRef.current[messageId]
    if (threadElement) {
      threadElement.scrollIntoView({ behavior: 'smooth', block: 'center' })
      return
    }
    if (parentMessageId) {
      const root = currentMessages.find((message) => message.id === parentMessageId)
      if (root) {
        void openThread(root).then(() => {
          window.requestAnimationFrame(() => threadReplyElementsRef.current[messageId]?.scrollIntoView({ behavior: 'smooth', block: 'center' }))
        })
        return
      }
    }
    if (!element) {
      setSummaryError(t('errors.sourceMissing'))
    }
  }

  const sendThreadReply = async () => {
    const body = threadDraft.trim()
    if (!body || !threadRoot) return
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    const requestChannelId = selectedChannelId
    const requestRootId = threadRoot.id
    setThreadDraft('')
    try {
      const reply = fromApiMessage(await chatApi.createMessage(requestChannelId, { body, parent_message_id: requestRootId }))
      if (selectedChannelRef.current !== requestChannelId || threadRootRef.current?.id !== requestRootId) return
      await enqueueRealtimeTask(realtimeQueueRef, () => addThreadReply(reply, requestChannelId, true))
    } catch {
      if (selectedChannelRef.current === requestChannelId && threadRootRef.current?.id === requestRootId) {
        setThreadDraft((current) => current.length === 0 ? body : current)
        setActionError(t('errors.replySend'))
      }
    }
  }

  const toggleSaved = (messageId: string) => {
    setSavedMessages((current) => {
      const exists = current.some((item) => item.channelId === selectedChannelId && item.messageId === messageId)
      if (exists) return current.filter((item) => !(item.channelId === selectedChannelId && item.messageId === messageId))
      return [...current, { channelId: selectedChannelId, messageId }]
    })
  }

  const onThreadKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => { if (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) return; if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); void sendThreadReply() } }
  const typingLabel = Object.values(typingUsers[selectedChannelId] ?? {}).join('、')
  const openThreadFromOverlay = (channel: Channel, message: Message) => {
    selectChannelAndResetJump(channel)
    void openThread(message)
  }

  const openSavedMessage = async (channel: Channel, messageId: string) => {
    setWorkspaceOverlay(null)
    setActionError(null)
    try {
      const loaded = (messages[channel.id] ?? []).some((message) => message.id === messageId)
        || await loadMessageUntil(channel.id, messageId)
      if (!loaded) {
        setActionError(t('errors.savedMessageMissing'))
        return
      }
      selectChannelAndResetJump(channel)
      setPendingSavedJump({ channelId: channel.id, messageId })
    } catch {
      setActionError(t('errors.savedMessageMissing'))
    }
  }

  if (authState !== 'authenticated' || !authUser) return <AuthScreen initialError={authState === 'unavailable' ? t('errors.apiUnavailable') : undefined} onAuthenticated={(user) => { setAuthUserWithRef(user); setAuthState('authenticated') }} />

  return (
    <div className={`app-shell ${showDetails ? 'app-shell-with-details' : ''}`}>
      <WorkspaceSidebar channels={channels} selectedChannelId={selectedChannelId} currentUser={currentUser} myPresence={myPresence} channelGroups={channelGroups} onSelectChannel={selectChannelAndResetJump} onAddChannel={openChannelCreate} onChangePresence={changePresence} onUpdateProfile={updateProfile} onLogout={() => void logout()} unreadCount={unreadCount} savedCount={savedMessages.length} threadCount={threadCount} onOpenSearch={() => setWorkspaceOverlay('search')} onOpenQuickLink={setWorkspaceOverlay} onOpenWorkspace={() => setWorkspaceOverlay('workspace')} onOpenHelp={() => setWorkspaceOverlay('help')} />
      {channelCreateGroup && <ChannelCreateDialog initialGroup={channelCreateGroup} groups={channelGroups} members={availableMembers} currentUserId={currentUser.id} onCreate={createChannel} onClose={() => setChannelCreateGroup(null)} />}
      {channelEditOpen && <ChannelEditDialog channel={selectedChannel} members={availableMembers} channelMembers={selectedChannelMembers} currentUserId={currentUser.id} currentUserRole={currentChannelRole} onSave={updateChannel} onClose={() => setChannelEditOpen(false)} />}
      <ChatPanel selectedChannel={selectedChannel} visibleMessages={visibleMessages} currentUser={currentUser} backendAvailable={backendReady} errorMessage={actionError ?? (backendUnavailable ? backendUnavailableMessage : undefined)} searchOpen={searchOpen} searchQuery={searchQuery} editingId={editingId} draft={draft} outgoingMessages={outgoingMessages} onRetryOutgoingMessage={(id) => void retryOutgoingMessage(id)} editDraft={editDraft} messageListRef={messageListRef} messageElementsRef={messageElementsRef} highlightedMessageId={highlightedMessageId} hasMore={messagePagination[selectedChannelId]?.hasMore ?? false} loadingOlder={messagePagination[selectedChannelId]?.loading ?? false} onLoadOlder={loadOlderMessages} onSearchOpenChange={setSearchOpen} onSearchQueryChange={setSearchQuery} onToggleDetails={() => setShowDetails((open) => !open)} canEditChannel={canEditSelectedChannel} onOpenChannelEdit={() => setChannelEditOpen(true)} onToggleReaction={toggleReaction} savedMessageIds={savedMessageIds} onToggleSaved={toggleSaved} onOpenThread={(message) => void openThread(message)} typingLabel={typingLabel} onStartEditing={startEditing} onDeleteMessage={(messageId) => void deleteMessage(messageId)} onDraftChange={onDraftChange} onEditDraftChange={setEditDraft} onComposerKeyDown={onComposerKeyDown} onSubmit={() => { if (editingId) void updateMessage(); else void sendMessage() }} onCancelEditing={() => { setEditingId(null); setEditDraft('') }} />
      {workspaceOverlay && <WorkspaceOverlay kind={workspaceOverlay} channels={channels} messages={messages} savedMessages={savedMessages} threadItems={workspaceThreadsLoaded ? workspaceThreadItems : undefined} memberCount={availableMembersLoaded ? availableMembers.length : undefined} connection={connection} onSelectChannel={selectChannelAndResetJump} onOpenSavedMessage={(channel, messageId) => void openSavedMessage(channel, messageId)} onOpenThread={openThreadFromOverlay} onClose={() => setWorkspaceOverlay(null)} />}
      {showDetails && <DetailsPanel selectedChannel={selectedChannel} members={detailMembers} summary={workSummary} summaryLoading={summaryLoading} summaryError={summaryError ?? undefined} connection={connection} onGenerateSummary={() => void generateSummary()} onJumpToMessage={jumpToMessage} onClose={() => setShowDetails(false)} />}
      {threadRoot && <ThreadPanel root={threadRoot} replies={threadReplies} draft={threadDraft} loading={threadLoading} hasMore={threadPagination.hasMore} loadingOlder={threadPagination.loading} showDetails={showDetails} onDraftChange={setThreadDraft} onKeyDown={onThreadKeyDown} onSubmit={() => void sendThreadReply()} onLoadOlder={loadOlderThreadReplies} onClose={closeThread} replyElementsRef={threadReplyElementsRef} />}
    </div>
  )
}

export default App
