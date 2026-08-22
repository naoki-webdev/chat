import { useEffect, useRef, useState, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi } from '../services/chatApi'
import { fromApiMessage, type Message } from '../types/chat'
import { t } from '../i18n'

type MessageMap = Record<string, Message[]>

type UseThreadOptions = {
  backendReady: boolean
  selectedChannelRef: MutableRefObject<string>
  messages: MessageMap
  setActionError: Dispatch<SetStateAction<string | null>>
}

export function useThread({ backendReady, selectedChannelRef, messages, setActionError }: UseThreadOptions) {
  const [threadRoot, setThreadRoot] = useState<Message | null>(null)
  const [threadReplies, setThreadReplies] = useState<Message[]>([])
  const [threadDraft, setThreadDraft] = useState('')
  const [threadLoading, setThreadLoading] = useState(false)
  const threadReplyIDsRef = useRef<Set<string>>(new Set())
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

  const closeThread = () => {
    invalidateRequest()
    threadRootRef.current = null
    setThreadRoot(null)
    setThreadReplies([])
    threadReplyIDsRef.current.clear()
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
    setThreadLoading(true)
    try {
      if (backendReady) {
        const page = await chatApi.listThreadMessages(message.id, undefined, 50, controller.signal)
        if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId || threadRootRef.current?.id !== requestRootId) return
        const loadedReplies = page.messages.map(fromApiMessage)
        threadReplyIDsRef.current = new Set(loadedReplies.map((reply) => reply.id))
        setThreadReplies(loadedReplies)
      } else {
        const loadedReplies = (messages[requestChannelId] ?? []).filter((item) => item.parentMessageId === message.id)
        if (requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId || threadRootRef.current?.id !== requestRootId) return
        threadReplyIDsRef.current = new Set(loadedReplies.map((reply) => reply.id))
        setThreadReplies(loadedReplies)
      }
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

  return {
    threadRoot,
    setThreadRoot,
    threadReplies,
    setThreadReplies,
    threadDraft,
    setThreadDraft,
    threadLoading,
    threadRootRef,
    threadReplyIDsRef,
    openThread,
    closeThread,
    invalidateRequest,
  }
}
