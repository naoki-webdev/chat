import { useRef, useState, type Dispatch, type KeyboardEvent as ReactKeyboardEvent, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi, type ApiMessage } from '../services/chatApi'
import { fromApiMessage, mergeMessage, type Message } from '../types/chat'
import { t } from '../i18n'
import { enqueueRealtimeTask, type RealtimeQueueRef } from './realtimeQueue'

type MessageMap = Record<string, Message[]>
type PaginationState = { nextCursor?: string; hasMore: boolean; loading: boolean }

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
  const [messagePagination, setMessagePagination] = useState<Record<string, PaginationState>>({})
  const messageListRef = useRef<HTMLDivElement>(null)
  const messageElementsRef = useRef<Record<string, HTMLElement | null>>({})
  const loadMessagesDirectRef = useRef<(channelId: string) => Promise<void>>(async () => undefined)
  const fullLoadSequenceRef = useRef<Record<string, number>>({})
  const fullLoadControllersRef = useRef<Record<string, AbortController | undefined>>({})
  const typingTimerRef = useRef<number | undefined>(undefined)
  const typingActiveRef = useRef(false)

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
    sendRealtime({ type: 'typing.stopped', channel_id: selectedChannelRef.current })
  }

  const onDraftChange = (value: string) => {
    setDraft(value)
    if (!backendReady || !value.trim()) {
      stopTyping()
      return
    }
    if (!typingActiveRef.current) {
      typingActiveRef.current = true
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

    try {
      const page = await chatApi.listMessages(channelId, before, 50, request?.controller.signal)
      if (request && (request.controller.signal.aborted || fullLoadSequenceRef.current[channelId] !== request.sequence)) return
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
      if (!request?.controller.signal.aborted) throw error
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

  const startEditing = (message: Message) => { setEditingId(message.id); setEditDraft(message.body); setDraft('') }
  const onComposerKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => { if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); if (editingId) void updateMessage(); else void sendMessage() } }

  return {
    draft,
    setDraft,
    editingId,
    setEditingId,
    editDraft,
    setEditDraft,
    messagePagination,
    messageListRef,
    messageElementsRef,
    loadMessages,
    loadMessagesDirectRef,
    loadOlderMessages,
    stopTyping,
    onDraftChange,
    sendMessage,
    updateMessage,
    deleteMessage,
    toggleReaction,
    startEditing,
    onComposerKeyDown,
  }
}
