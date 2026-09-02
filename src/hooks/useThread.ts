import { useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi } from '../services/chatApi'
import { fromApiMessage, type Message } from '../types/chat'
import { t } from '../i18n'
import { enqueueRealtimeTask, type RealtimeQueueRef } from './realtimeQueue'

type MessageMap = Record<string, Message[]>
type ThreadPaginationState = { nextCursor?: string; hasMore: boolean; loading: boolean }

type UseThreadOptions = {
  backendReady: boolean
  selectedChannelRef: MutableRefObject<string>
  messages: MessageMap
  setActionError: Dispatch<SetStateAction<string | null>>
  realtimeQueueRef: RealtimeQueueRef
}

export function useThread({ backendReady, selectedChannelRef, messages, setActionError, realtimeQueueRef }: UseThreadOptions) {
  const [threadRoot, setThreadRoot] = useState<Message | null>(null)
  const [threadReplies, setThreadReplies] = useState<Message[]>([])
  const [threadDraft, setThreadDraft] = useState('')
  const [threadLoading, setThreadLoading] = useState(false)
  const [threadPagination, setThreadPagination] = useState<ThreadPaginationState>({ hasMore: false, loading: false })
  const threadReplyIDsRef = useRef<Set<string>>(new Set())
  const threadReplyElementsRef = useRef<Record<string, HTMLElement | null>>({})
  const threadPaginationRef = useRef<ThreadPaginationState>({ hasMore: false, loading: false })
  const threadRootRef = useRef<Message | null>(null)
  const requestSequenceRef = useRef(0)
  const abortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => () => {
    requestSequenceRef.current += 1
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
  }, [])

  const invalidateRequest = () => {
    requestSequenceRef.current += 1
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
  }

  const loadThreadReplies = async (
    message: Message,
    requestChannelId: string,
    requestRootId: string,
    requestSequence: number,
    controller: AbortController,
  ) => {
    let loadedReplies: Message[]
    let pagination: ThreadPaginationState
    if (backendReady) {
      const page = await chatApi.listThreadMessages(message.id, undefined, 50, controller.signal)
      if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId || threadRootRef.current?.id !== requestRootId) return
      loadedReplies = page.messages.map(fromApiMessage)
      pagination = { nextCursor: page.next_cursor, hasMore: page.has_more, loading: false }
    } else {
      loadedReplies = (messages[requestChannelId] ?? []).filter((item) => item.parentMessageId === message.id)
      if (requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId || threadRootRef.current?.id !== requestRootId) return
      pagination = { hasMore: false, loading: false }
    }
    threadReplyIDsRef.current = new Set(loadedReplies.map((reply) => reply.id))
    setThreadReplies(loadedReplies)
    threadPaginationRef.current = pagination
    setThreadPagination(pagination)
  }

  const closeThread = () => {
    invalidateRequest()
    threadRootRef.current = null
    setThreadRoot(null)
    setThreadReplies([])
    threadReplyIDsRef.current.clear()
    threadReplyElementsRef.current = {}
    threadPaginationRef.current = { hasMore: false, loading: false }
    setThreadPagination({ hasMore: false, loading: false })
    setThreadLoading(false)
  }

  const openThread = async (message: Message) => {
    invalidateRequest()
    const requestSequence = requestSequenceRef.current
    const requestChannelId = selectedChannelRef.current
    const requestRootId = message.id
    const controller = new AbortController()
    abortControllerRef.current = controller
    threadRootRef.current = message
    setThreadRoot(message)
    setThreadDraft('')
    threadPaginationRef.current = { hasMore: false, loading: false }
    setThreadPagination({ hasMore: false, loading: false })
    setThreadLoading(true)
    try {
      await enqueueRealtimeTask(realtimeQueueRef, () => loadThreadReplies(message, requestChannelId, requestRootId, requestSequence, controller))
    } catch {
      if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId || threadRootRef.current?.id !== requestRootId) return
      setActionError(t('errors.threadLoad'))
    } finally {
      if (requestSequence === requestSequenceRef.current) {
        abortControllerRef.current = null
        setThreadLoading(false)
      }
    }
  }

  const loadOlderThreadReplies = () => {
    const root = threadRootRef.current
    const pagination = threadPaginationRef.current
    if (!backendReady || !root || !pagination.hasMore || !pagination.nextCursor || pagination.loading) return
    const requestSequence = requestSequenceRef.current
    const requestChannelId = selectedChannelRef.current
    const requestRootId = root.id
    const controller = new AbortController()
    abortControllerRef.current = controller
    const loadingPagination = { ...pagination, loading: true }
    threadPaginationRef.current = loadingPagination
    setThreadPagination(loadingPagination)

    void enqueueRealtimeTask(realtimeQueueRef, async () => {
      const page = await chatApi.listThreadMessages(root.id, pagination.nextCursor, 50, controller.signal)
      if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId || threadRootRef.current?.id !== requestRootId) return
      const loadedReplies = page.messages.map(fromApiMessage)
      threadReplyIDsRef.current = new Set([...threadReplyIDsRef.current, ...loadedReplies.map((reply) => reply.id)])
      setThreadReplies((current) => {
        const currentIDs = new Set(current.map((reply) => reply.id))
        return [...loadedReplies.filter((reply) => !currentIDs.has(reply.id)), ...current]
      })
      const nextPagination = { nextCursor: page.next_cursor, hasMore: page.has_more, loading: false }
      threadPaginationRef.current = nextPagination
      setThreadPagination(nextPagination)
    }).catch(() => {
      if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId || threadRootRef.current?.id !== requestRootId) return
      const failedPagination = { ...threadPaginationRef.current, loading: false }
      threadPaginationRef.current = failedPagination
      setThreadPagination(failedPagination)
      setActionError(t('errors.threadLoad'))
    }).finally(() => {
      if (requestSequence === requestSequenceRef.current && abortControllerRef.current === controller) abortControllerRef.current = null
    })
  }

  return {
    threadRoot,
    setThreadRoot,
    threadReplies,
    setThreadReplies,
    threadDraft,
    setThreadDraft,
    threadLoading,
    threadPagination,
    threadRootRef,
    threadReplyIDsRef,
    threadReplyElementsRef,
    openThread,
    loadOlderThreadReplies,
    closeThread,
    invalidateRequest,
  }
}
