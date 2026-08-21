import { useCallback, type Dispatch, type SetStateAction } from 'react'
import { chatApi, ChatApiError, type ApiChannelMember } from '../services/chatApi'
import { fromApiChannel, type Channel, type Message } from '../types/chat'
import { t } from '../i18n'

type ChannelCreatePayload = {
  name: string
  group: string
  description: string
  memberIds: string[]
}

type ChannelUpdatePayload = {
  name: string
  description: string
  memberIds: string[]
}

type UseChannelManagementOptions = {
  backendReady: boolean
  backendUnavailableMessage: string
  selectedChannelId: string
  setChannels: Dispatch<SetStateAction<Channel[]>>
  setMessages: Dispatch<SetStateAction<Record<string, Message[]>>>
  setSelectedChannelId: Dispatch<SetStateAction<string>>
  setSelectedChannelMembers: Dispatch<SetStateAction<ApiChannelMember[]>>
  setChannelCreateGroup: Dispatch<SetStateAction<string | null>>
  setChannelEditOpen: Dispatch<SetStateAction<boolean>>
  setActionError: Dispatch<SetStateAction<string | null>>
}

export function useChannelManagement({
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
}: UseChannelManagementOptions) {
  const openChannelCreate = useCallback((group: string) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      return
    }
    setActionError(null)
    setChannelCreateGroup(group)
  }, [backendReady, backendUnavailableMessage, setActionError, setChannelCreateGroup])

  const createChannel = useCallback(async (payload: ChannelCreatePayload) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      throw new Error('backend unavailable')
    }
    try {
      const channel = fromApiChannel(await chatApi.createChannel({ name: payload.name, group: payload.group, kind: 'channel', description: payload.description, member_ids: payload.memberIds }))
      setChannels((current) => [...current, channel])
      setMessages((current) => ({ ...current, [channel.id]: [] }))
      setSelectedChannelId(channel.id)
      setChannelCreateGroup(null)
      setActionError(null)
    } catch (error) {
      setActionError(error instanceof ChatApiError && error.status === 409 ? t('errors.channelConflict') : t('errors.channelCreate'))
      throw new Error('channel creation failed')
    }
  }, [backendReady, backendUnavailableMessage, setActionError, setChannelCreateGroup, setChannels, setMessages, setSelectedChannelId])

  const updateChannel = useCallback(async (payload: ChannelUpdatePayload) => {
    if (!backendReady) {
      setActionError(backendUnavailableMessage)
      throw new Error('backend unavailable')
    }
    try {
      const channel = fromApiChannel(await chatApi.updateChannel(selectedChannelId, { name: payload.name, description: payload.description, member_ids: payload.memberIds }))
      setChannels((current) => current.map((item) => item.id === channel.id ? channel : item))
      try {
        const memberResponse = await chatApi.listChannelMembers(selectedChannelId)
        setSelectedChannelMembers(memberResponse.members)
      } catch {
        // The channel update already succeeded; keep the current member view until the next refresh.
      }
      setChannelEditOpen(false)
      setActionError(null)
    } catch {
      setActionError(t('errors.channelUpdate'))
      throw new Error('channel update failed')
    }
  }, [backendReady, backendUnavailableMessage, selectedChannelId, setActionError, setChannelEditOpen, setChannels, setSelectedChannelMembers])

  return { openChannelCreate, createChannel, updateChannel }
}
