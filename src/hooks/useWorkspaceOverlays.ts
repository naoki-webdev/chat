import { useEffect, useMemo, useState, type MutableRefObject } from 'react'
import { chatApi } from '../services/chatApi'
import { fromApiMessage, type Message } from '../types/chat'
import type { MessageMap } from '../types/messageState'
import type { WorkspaceOverlayKind } from '../components/WorkspaceOverlay'

type WorkspaceThreadItem = { channelId: string; message: Message }

type Props = {
  backendReady: boolean
  selectedChannelId: string
  selectedChannelRef: MutableRefObject<string>
  messages: MessageMap
  searchQuery: string
}

export function useWorkspaceOverlays({ backendReady, selectedChannelId, selectedChannelRef, messages, searchQuery }: Props) {
  const [serverSearchResults, setServerSearchResults] = useState<Message[] | null>(null)
  const [workspaceOverlay, setWorkspaceOverlay] = useState<WorkspaceOverlayKind | null>(null)
  const [workspaceThreadItems, setWorkspaceThreadItems] = useState<WorkspaceThreadItem[]>([])
  const [workspaceThreadCount, setWorkspaceThreadCount] = useState(0)
  const [workspaceThreadsLoaded, setWorkspaceThreadsLoaded] = useState(false)

  const currentMessages = useMemo(() => messages[selectedChannelId] ?? [], [messages, selectedChannelId])

  useEffect(() => {
    const query = searchQuery.trim()
    if (!backendReady || !query) {
      setServerSearchResults(null)
      return
    }
    const controller = new AbortController()
    setServerSearchResults(null)
    void chatApi.searchMessages(selectedChannelId, query, 100, controller.signal)
      .then((page) => {
        if (!controller.signal.aborted && selectedChannelRef.current === selectedChannelId) setServerSearchResults(page.messages.map(fromApiMessage))
      })
      .catch(() => {
        if (!controller.signal.aborted) setServerSearchResults(null)
      })
    return () => controller.abort()
  }, [backendReady, searchQuery, selectedChannelId, selectedChannelRef])

  const visibleMessages = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    if (!query) return currentMessages
    if (serverSearchResults) return serverSearchResults
    return currentMessages.filter((message) => `${message.author} ${message.body}`.toLowerCase().includes(query))
  }, [currentMessages, searchQuery, serverSearchResults])

  useEffect(() => {
    if (!backendReady || workspaceOverlay !== 'threads') return
    const controller = new AbortController()
    setWorkspaceThreadsLoaded(false)
    void chatApi.listThreadRoots(100, controller.signal)
      .then((page) => {
        if (controller.signal.aborted) return
        setWorkspaceThreadItems(page.messages.map((message) => ({ channelId: message.channel_id, message: fromApiMessage(message) })))
        setWorkspaceThreadCount(page.total)
        setWorkspaceThreadsLoaded(true)
      })
      .catch(() => {
        if (!controller.signal.aborted) setWorkspaceThreadsLoaded(false)
      })
    return () => controller.abort()
  }, [backendReady, workspaceOverlay])

  return { currentMessages, visibleMessages, workspaceOverlay, setWorkspaceOverlay, workspaceThreadItems, workspaceThreadCount, workspaceThreadsLoaded }
}
