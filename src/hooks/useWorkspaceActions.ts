import { useCallback, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi, type ApiUser } from '../services/chatApi'
import { t } from '../i18n'
import { updateMessagesByAuthor, type MessageMap } from '../types/messageState'
import type { Channel, Message } from '../types/chat'

type AuthState = 'checking' | 'anonymous' | 'authenticated' | 'unavailable'
type Presence = NonNullable<Channel['presence']>

type UseWorkspaceActionsOptions = {
  backendReady: boolean
  backendUnavailableMessage: string
  currentUser: ApiUser
  selectedChannelRef: MutableRefObject<string>
  threadRootRef: MutableRefObject<Message | null>
  threadReplyIDsRef: MutableRefObject<Set<string>>
  stopTyping: () => void
  invalidateThreadRequest: () => void
  sendPresence: (presence: Presence) => void
  setSelectedChannelId: Dispatch<SetStateAction<string>>
  setSearchQuery: Dispatch<SetStateAction<string>>
  setChannelEditOpen: Dispatch<SetStateAction<boolean>>
  setChannels: Dispatch<SetStateAction<Channel[]>>
  setMessages: Dispatch<SetStateAction<MessageMap>>
  setAuthUser: Dispatch<SetStateAction<ApiUser | null>>
  setAuthState: Dispatch<SetStateAction<AuthState>>
  setBackendState: Dispatch<SetStateAction<'checking' | 'ready' | 'unavailable'>>
  setMyPresence: Dispatch<SetStateAction<Presence>>
  setThreadRoot: Dispatch<SetStateAction<Message | null>>
  setThreadReplies: Dispatch<SetStateAction<Message[]>>
  setActionError: Dispatch<SetStateAction<string | null>>
}

export function useWorkspaceActions({
  backendReady,
  backendUnavailableMessage,
  currentUser,
  selectedChannelRef,
  threadRootRef,
  threadReplyIDsRef,
  stopTyping,
  invalidateThreadRequest,
  sendPresence,
  setSelectedChannelId,
  setSearchQuery,
  setChannelEditOpen,
  setChannels,
  setMessages,
  setAuthUser,
  setAuthState,
  setBackendState,
  setMyPresence,
  setThreadRoot,
  setThreadReplies,
  setActionError,
}: UseWorkspaceActionsOptions) {
  const selectChannel = useCallback((channel: Channel) => {
    stopTyping()
    invalidateThreadRequest()
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
  }, [backendReady, invalidateThreadRequest, selectedChannelRef, setActionError, setChannelEditOpen, setChannels, setSearchQuery, setSelectedChannelId, setThreadReplies, setThreadRoot, stopTyping, threadReplyIDsRef, threadRootRef])

  const changePresence = useCallback((nextPresence: Presence) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    setMyPresence(nextPresence)
    sendPresence(nextPresence)
    setActionError(null)
  }, [backendReady, backendUnavailableMessage, sendPresence, setActionError, setMyPresence])

  const updateProfile = useCallback(async (name: string) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      throw new Error('backend unavailable')
    }
    try {
      const updatedUser = await chatApi.updateProfile(name)
      setAuthUser(updatedUser)
      setMessages((current) => updateMessagesByAuthor(current, currentUser.id, {
        author: updatedUser.name,
        initials: updatedUser.initials,
        color: updatedUser.color,
      }))
      setThreadRoot((current) => current?.authorID === currentUser.id
        ? { ...current, author: updatedUser.name, initials: updatedUser.initials, color: updatedUser.color }
        : current)
      setThreadReplies((current) => current.map((message) => message.authorID === currentUser.id
        ? { ...message, author: updatedUser.name, initials: updatedUser.initials, color: updatedUser.color }
        : message))
      setActionError(null)
    } catch (error) {
      setActionError(t('errors.profileUpdate'))
      throw error
    }
  }, [backendReady, backendUnavailableMessage, currentUser.id, setActionError, setAuthUser, setMessages, setThreadReplies, setThreadRoot])

  const logout = useCallback(async () => {
    try {
      await chatApi.logout()
    } finally {
      setAuthUser(null)
      setAuthState('anonymous')
      setBackendState('checking')
    }
  }, [setAuthState, setAuthUser, setBackendState])

  return { selectChannel, changePresence, updateProfile, logout }
}
