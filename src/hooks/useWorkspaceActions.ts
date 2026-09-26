import { useCallback, useRef, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { chatApi, type ApiMember, type ApiUser } from '../services/chatApi'
import { t } from '../i18n'
import { updateMessagesByAuthor, type MessageMap } from '../types/messageState'
import type { Channel, Message } from '../types/chat'
import { initialChannels, initialMessages } from '../types/demoData'
import type { WorkspaceOverlayKind } from '../components/WorkspaceOverlay'

type AuthState = 'checking' | 'anonymous' | 'authenticated' | 'unavailable'
type Presence = NonNullable<Channel['presence']>

type UseWorkspaceActionsOptions = {
  backendReady: boolean
  backendUnavailableMessage: string
  currentUser: ApiUser
  selectedChannelRef: MutableRefObject<string>
  threadRootRef: MutableRefObject<Message | null>
  threadReplyIDsRef: MutableRefObject<Set<string>>
  invalidateMessageRequests: () => void
  invalidateChannelRefresh: () => void
  clearScheduledChannelReads: () => void
  stopTyping: () => void
  invalidateThreadRequest: () => void
  sendPresence: (presence: Presence) => void
  setSelectedChannelId: Dispatch<SetStateAction<string>>
  setChannelGroups?: Dispatch<SetStateAction<string[]>>
  setDraft: Dispatch<SetStateAction<string>>
  setEditingId: Dispatch<SetStateAction<string | null>>
  setEditDraft: Dispatch<SetStateAction<string>>
  setSearchOpen: Dispatch<SetStateAction<boolean>>
  setSearchQuery: Dispatch<SetStateAction<string>>
  setWorkspaceOverlay: Dispatch<SetStateAction<WorkspaceOverlayKind | null>>
  setChannelEditOpen: Dispatch<SetStateAction<boolean>>
  setChannelCreateGroup: Dispatch<SetStateAction<string | null>>
  setAvailableMembers: Dispatch<SetStateAction<ApiMember[]>>
  setAvailableMembersLoaded: Dispatch<SetStateAction<boolean>>
  setChannels: Dispatch<SetStateAction<Channel[]>>
  setMessages: Dispatch<SetStateAction<MessageMap>>
  setAuthUser: Dispatch<SetStateAction<ApiUser | null>>
  setAuthState: Dispatch<SetStateAction<AuthState>>
  setBackendState: Dispatch<SetStateAction<'checking' | 'ready' | 'unavailable'>>
  setMyPresence: Dispatch<SetStateAction<Presence>>
  setTypingUsers: Dispatch<SetStateAction<Record<string, Record<string, string>>>>
  setThreadDraft: Dispatch<SetStateAction<string>>
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
  invalidateMessageRequests,
  invalidateChannelRefresh,
  clearScheduledChannelReads,
  stopTyping,
  invalidateThreadRequest,
  sendPresence,
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
  setAuthUser,
  setAuthState,
  setBackendState,
  setMyPresence,
  setTypingUsers,
  setThreadDraft,
  setThreadRoot,
  setThreadReplies,
  setActionError,
}: UseWorkspaceActionsOptions) {
  const profileRequestSequenceRef = useRef(0)

  const selectChannel = useCallback((channel: Channel) => {
    stopTyping()
    invalidateThreadRequest()
    selectedChannelRef.current = channel.id
    setSelectedChannelId(channel.id)
    setDraft('')
    setEditingId(null)
    setEditDraft('')
    threadRootRef.current = null
    setThreadRoot(null)
    setThreadReplies([])
    setThreadDraft('')
    threadReplyIDsRef.current.clear()
    setSearchQuery('')
    setChannelEditOpen(false)
    if (backendReady) {
      setChannels((current) => current.map((item) => item.id === channel.id ? { ...item, unread: 0 } : item))
    }
  }, [backendReady, invalidateThreadRequest, selectedChannelRef, setActionError, setChannelEditOpen, setChannels, setDraft, setEditDraft, setEditingId, setSearchQuery, setSelectedChannelId, setThreadDraft, setThreadReplies, setThreadRoot, stopTyping, threadReplyIDsRef, threadRootRef])

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
    const requestSequence = ++profileRequestSequenceRef.current
    try {
      const updatedUser = await chatApi.updateProfile(name)
      if (requestSequence !== profileRequestSequenceRef.current) return
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
      if (requestSequence === profileRequestSequenceRef.current) setActionError(t('errors.profileUpdate'))
      throw error
    }
  }, [backendReady, backendUnavailableMessage, currentUser.id, setActionError, setAuthUser, setMessages, setThreadReplies, setThreadRoot])

  const logout = useCallback(async () => {
    try {
      await chatApi.logout()
    } catch {
      setActionError(t('errors.logout'))
      return false
    }

    profileRequestSequenceRef.current += 1
    invalidateMessageRequests()
    invalidateChannelRefresh()
    clearScheduledChannelReads()
    stopTyping()
    invalidateThreadRequest()
    setChannelCreateGroup(null)
    setAvailableMembers([])
    setAvailableMembersLoaded(false)
    selectedChannelRef.current = 'design-system'
    setSelectedChannelId('design-system')
    setDraft('')
    setChannels(initialChannels)
    setChannelGroups?.(['Engineering', 'Product'])
    setMessages(initialMessages)
    threadRootRef.current = null
    setThreadRoot(null)
    setThreadReplies([])
    setThreadDraft('')
    threadReplyIDsRef.current.clear()
    setEditingId(null)
    setEditDraft('')
    setSearchOpen(false)
    setSearchQuery('')
    setWorkspaceOverlay(null)
    setChannelEditOpen(false)
    setTypingUsers({})
    setMyPresence('online')
    setActionError(null)
    setAuthUser(null)
    setAuthState('anonymous')
    setBackendState('checking')
    return true
  }, [clearScheduledChannelReads, invalidateChannelRefresh, invalidateMessageRequests, invalidateThreadRequest, selectedChannelRef, setActionError, setAuthState, setAuthUser, setAvailableMembers, setAvailableMembersLoaded, setBackendState, setChannelCreateGroup, setChannelEditOpen, setChannelGroups, setChannels, setDraft, setEditDraft, setEditingId, setMessages, setMyPresence, setSearchOpen, setSearchQuery, setSelectedChannelId, setThreadDraft, setThreadReplies, setThreadRoot, setTypingUsers, setWorkspaceOverlay, stopTyping, threadReplyIDsRef, threadRootRef])

  return { selectChannel, changePresence, updateProfile, logout }
}
