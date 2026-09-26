import { useCallback, useEffect, useRef, useState, type Dispatch, type KeyboardEvent as ReactKeyboardEvent, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi, type ApiMessage } from '../services/chatApi'
import { fromApiMessage, type Message } from '../types/chat'
import { type MessageMap, upsertMessageInMap } from '../types/messageState'
import { t } from '../i18n'
import { enqueueRealtimeTask, type RealtimeQueueRef } from './realtimeQueue'

type PaginationState = { nextCursor?: string; hasMore: boolean; loading: boolean }
export type OutgoingMessage = { id: number; channelId: string; body: string; status: 'sending' | 'failed' }

type UseChatMessagesOptions = {
  backendReady: boolean
  backendUnavailableMessage: string
  selectedChannelId: string
  selectedChannelRef: MutableRefObject<string>
  messages: MessageMap
  setMessages: Dispatch<SetStateAction<MessageMap>>
  setActionError: Dispatch<SetStateAction<string | null>>
  setThreadReplies: Dispatch<SetStateAction<Message[]>>
  advanceEventCursorRef: MutableRefObject<(cursor: number) => void>
  sendRealtime: (payload: unknown) => void
  realtimeQueueRef: RealtimeQueueRef
}

export function useChatMessages({
  backendReady,
  backendUnavailableMessage,
  selectedChannelId,
  selectedChannelRef,
  messages,
  setMessages,
  setActionError,
  setThreadReplies,
  advanceEventCursorRef,
  sendRealtime,
  realtimeQueueRef,
}: UseChatMessagesOptions) {
  const [draft, setDraft] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState('')
  const [outgoingMessages, setOutgoingMessages] = useState<OutgoingMessage[]>([])
  const [messagePagination, setMessagePagination] = useState<Record<string, PaginationState>>({})
  const messageListRef = useRef<HTMLDivElement>(null)
  const messageElementsRef = useRef<Record<string, HTMLElement | null>>({})
  const loadMessagesDirectRef = useRef<(channelId: string) => Promise<void>>(async () => undefined)
  const fullLoadSequenceRef = useRef<Record<string, number>>({})
  const fullLoadControllersRef = useRef<Record<string, AbortController | undefined>>({})
  const messageRequestGenerationRef = useRef(0)
  const messageActionGenerationRef = useRef(0)
  const outgoingSequenceRef = useRef(0)
  const outgoingRequestsRef = useRef(new Set<number>())
  const typingTimerRef = useRef<number | undefined>(undefined)
  const typingActiveRef = useRef(false)
  const typingChannelRef = useRef<string | null>(null)
  const editingIdRef = useRef<string | null>(null)
  editingIdRef.current = editingId

  const invalidateMessageRequests = useCallback(() => {
    messageRequestGenerationRef.current += 1
    messageActionGenerationRef.current += 1
    outgoingRequestsRef.current.clear()
    setOutgoingMessages([])
    Object.values(fullLoadControllersRef.current).forEach((controller) => controller?.abort())
    Object.keys(fullLoadSequenceRef.current).forEach((channelId) => {
      fullLoadSequenceRef.current[channelId] += 1
    })
    fullLoadControllersRef.current = {}
  }, [])

  useEffect(() => () => invalidateMessageRequests(), [invalidateMessageRequests])

  const beginFullLoad = (channelId: string) => {
    const sequence = (fullLoadSequenceRef.current[channelId] ?? 0) + 1
    const controller = new AbortController()
    fullLoadSequenceRef.current[channelId] = sequence
    fullLoadControllersRef.current[channelId]?.abort()
    fullLoadControllersRef.current[channelId] = controller
    return { sequence, controller }
  }

  const stopTyping = () => {
    if (!typingActiveRef.current) return
    typingActiveRef.current = false
    if (typingTimerRef.current) window.clearTimeout(typingTimerRef.current)
    typingTimerRef.current = undefined
    const channelId = typingChannelRef.current
    typingChannelRef.current = null
    if (channelId) sendRealtime({ type: 'typing.stopped', channel_id: channelId })
  }

  const onDraftChange = (value: string) => {
    setDraft(value)
    if (!backendReady || !value.trim()) {
      stopTyping()
      return
    }
    if (!typingActiveRef.current) {
      typingActiveRef.current = true
      typingChannelRef.current = selectedChannelId
      sendRealtime({ type: 'typing.started', channel_id: selectedChannelId })
    }
    if (typingTimerRef.current) window.clearTimeout(typingTimerRef.current)
    typingTimerRef.current = window.setTimeout(stopTyping, 1500)
  }

  const loadMessagesPage = async (
    channelId: string,
    before?: string,
    fullLoadRequest?: { sequence: number; controller: AbortController },
  ) => {
    const request = before ? undefined : fullLoadRequest ?? beginFullLoad(channelId)
    const requestGeneration = messageRequestGenerationRef.current
    if (request?.controller.signal.aborted) return

    try {
      const page = await chatApi.listMessages(channelId, before, 50, request?.controller.signal)
      if (requestGeneration !== messageRequestGenerationRef.current || (request && (request.controller.signal.aborted || fullLoadSequenceRef.current[channelId] !== request.sequence))) return
      advanceEventCursorRef.current(page.cursor)
      setMessages((current) => {
        const incoming = page.messages.filter((message) => !message.parent_message_id).map(fromApiMessage)
        if (!before) return { ...current, [channelId]: incoming }
        const existing = current[channelId] ?? []
        const existingIDs = new Set(existing.map((message) => message.id))
        return { ...current, [channelId]: [...incoming.filter((message) => !existingIDs.has(message.id)), ...existing] }
      })
      setMessagePagination((current) => ({ ...current, [channelId]: { nextCursor: page.next_cursor, hasMore: page.has_more, loading: false } }))
    } catch (error) {
      if (requestGeneration === messageRequestGenerationRef.current && !request?.controller.signal.aborted) throw error
    } finally {
      if (request && fullLoadControllersRef.current[channelId] === request.controller) {
        delete fullLoadControllersRef.current[channelId]
      }
    }
  }

  const loadMessages = (channelId: string, before?: string) => {
    if (before) return loadMessagesPage(channelId, before)
    const request = beginFullLoad(channelId)
    return enqueueRealtimeTask(realtimeQueueRef, () => loadMessagesPage(channelId, undefined, request))
  }

  loadMessagesDirectRef.current = (channelId) => {
    const request = beginFullLoad(channelId)
    return loadMessagesPage(channelId, undefined, request)
  }

  const loadOlderMessages = () => {
    const pagination = messagePagination[selectedChannelId]
    if (!backendReady || !pagination?.hasMore || !pagination.nextCursor || pagination.loading) return
    setMessagePagination((current) => ({ ...current, [selectedChannelId]: { ...pagination, loading: true } }))
    void loadMessages(selectedChannelId, pagination.nextCursor).catch(() => setMessagePagination((current) => ({ ...current, [selectedChannelId]: { ...pagination, loading: false } })))
  }

  const loadMessageUntil = async (channelId: string, messageId: string) => {
    if (!backendReady) return false
    if ((messages[channelId] ?? []).some((message) => message.id === messageId)) return true

    const request = beginFullLoad(channelId)
    const requestGeneration = messageRequestGenerationRef.current
    const seenCursors = new Set<string>()
    let before: string | undefined

    try {
      for (let pageNumber = 0; pageNumber < 200; pageNumber += 1) {
        if (request.controller.signal.aborted || requestGeneration !== messageRequestGenerationRef.current) return false
        const page = await chatApi.listMessages(channelId, before, 50, request.controller.signal)
        if (request.controller.signal.aborted || requestGeneration !== messageRequestGenerationRef.current || fullLoadSequenceRef.current[channelId] !== request.sequence) return false
        const incoming = page.messages.filter((message) => !message.parent_message_id).map(fromApiMessage)
        const isOlderPage = before !== undefined
        setMessages((current) => {
          const existing = current[channelId] ?? []
          const existingIDs = new Set(existing.map((message) => message.id))
          return {
            ...current,
            [channelId]: isOlderPage
              ? [...incoming.filter((message) => !existingIDs.has(message.id)), ...existing]
              : incoming,
          }
        })
        setMessagePagination((current) => ({ ...current, [channelId]: { nextCursor: page.next_cursor, hasMore: page.has_more, loading: false } }))
        if (page.messages.some((message) => message.id === messageId)) return true
        const nextCursor = page.next_cursor
        if (!page.has_more || !nextCursor || seenCursors.has(nextCursor)) return false
        seenCursors.add(nextCursor)
        before = nextCursor
      }
      return false
    } finally {
      if (fullLoadControllersRef.current[channelId] === request.controller) {
        delete fullLoadControllersRef.current[channelId]
      }
    }
  }

  const upsertMessage = (remoteMessage: ApiMessage) => {
    const incoming = fromApiMessage(remoteMessage)
    setMessages((current) => upsertMessageInMap(current, remoteMessage.channel_id, incoming))
  }

  const sendOutgoingMessage = async (outgoing: OutgoingMessage) => {
    if (outgoingRequestsRef.current.has(outgoing.id)) return
    outgoingRequestsRef.current.add(outgoing.id)
    const actionGeneration = messageActionGenerationRef.current
    setOutgoingMessages((current) => current.map((item) => item.id === outgoing.id ? { ...item, status: 'sending' } : item))
    try {
      const created = await chatApi.createMessage(outgoing.channelId, { body: outgoing.body })
      if (actionGeneration !== messageActionGenerationRef.current) return
      upsertMessage(created)
      setOutgoingMessages((current) => current.filter((item) => item.id !== outgoing.id))
      if (selectedChannelRef.current === outgoing.channelId) setActionError(null)
    } catch {
      if (actionGeneration === messageActionGenerationRef.current) {
        setOutgoingMessages((current) => current.map((item) => item.id === outgoing.id ? { ...item, status: 'failed' } : item))
        if (selectedChannelRef.current === outgoing.channelId) setActionError(t('errors.messageSend'))
      }
    } finally {
      outgoingRequestsRef.current.delete(outgoing.id)
    }
  }

  const sendMessage = async () => {
    const body = draft.trim()
    if (!body) return
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    const channelId = selectedChannelId
    stopTyping()
    setDraft('')
    const outgoing: OutgoingMessage = { id: ++outgoingSequenceRef.current, channelId, body, status: 'sending' }
    setOutgoingMessages((current) => [...current, outgoing])
    await sendOutgoingMessage(outgoing)
    window.requestAnimationFrame(() => {
      if (selectedChannelRef.current !== channelId) return
      const list = messageListRef.current
      if (list) list.scrollTop = list.scrollHeight
    })
  }

  const retryOutgoingMessage = async (id: number) => {
    const outgoing = outgoingMessages.find((item) => item.id === id && item.status === 'failed')
    if (outgoing) await sendOutgoingMessage(outgoing)
  }

  const updateMessage = async () => {
    const body = editDraft.trim()
    if (!editingId || !body) return
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    const channelId = selectedChannelId
    const messageId = editingId
    const actionGeneration = messageActionGenerationRef.current
    try {
      const updated = await chatApi.updateMessage(editingId, body)
      if (actionGeneration !== messageActionGenerationRef.current) return
      upsertMessage(updated)
      if (selectedChannelRef.current === channelId) setActionError(null)
    } catch {
      if (actionGeneration === messageActionGenerationRef.current && selectedChannelRef.current === channelId) setActionError(t('errors.messageEdit'))
      return
    }
    if (actionGeneration === messageActionGenerationRef.current && selectedChannelRef.current === channelId && editingIdRef.current === messageId) {
      setEditingId(null)
      setEditDraft('')
    }
  }

  const deleteMessage = async (messageId: string) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    const channelId = selectedChannelId
    const actionGeneration = messageActionGenerationRef.current
    try {
      await chatApi.deleteMessage(messageId)
      if (actionGeneration !== messageActionGenerationRef.current) return
      await loadMessages(channelId)
      if (actionGeneration !== messageActionGenerationRef.current) return
      setThreadReplies((current) => current.filter((message) => message.id !== messageId))
      if (actionGeneration === messageActionGenerationRef.current && selectedChannelRef.current === channelId) setActionError(null)
    } catch {
      if (actionGeneration === messageActionGenerationRef.current && selectedChannelRef.current === channelId) setActionError(t('errors.messageDelete'))
    }
  }

  const toggleReaction = async (messageId: string, emoji: string) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    const message = (messages[selectedChannelId] ?? []).find((item) => item.id === messageId)
    const existing = message?.reactions?.find((reaction) => reaction.emoji === emoji)
    const actionGeneration = messageActionGenerationRef.current
    try {
      const updated = await (existing?.reacted ? chatApi.removeReaction(messageId, emoji) : chatApi.addReaction(messageId, emoji))
      if (actionGeneration !== messageActionGenerationRef.current) return
      upsertMessage(updated)
    } catch {
      if (actionGeneration === messageActionGenerationRef.current && selectedChannelRef.current === selectedChannelId) setActionError(t('errors.reactionUpdate'))
    }
  }

  const startEditing = (message: Message) => { setEditingId(message.id); setEditDraft(message.body); setDraft('') }
  const onComposerKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => { if (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) return; if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); if (editingId) void updateMessage(); else void sendMessage() } }

  return {
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
  }
}
