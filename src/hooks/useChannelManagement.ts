import { useCallback, type Dispatch, type SetStateAction } from 'react'
import { chatApi, ChatApiError } from '../services/chatApi'
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
  refreshSelectedChannelMembers: (channelId?: string) => Promise<void>
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
  refreshSelectedChannelMembers,
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
      throw Object.assign(new Error('channel creation failed'), { cause: error })
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
        await refreshSelectedChannelMembers(channel.id)
      } catch {
        // The channel update already succeeded; keep the current member view until the next refresh.
      }
      setChannelEditOpen(false)
      setActionError(null)
    } catch (error) {
      setActionError(t('errors.channelUpdate'))
      throw Object.assign(new Error('channel update failed'), { cause: error })
    }
  }, [backendReady, backendUnavailableMessage, refreshSelectedChannelMembers, selectedChannelId, setActionError, setChannelEditOpen, setChannels])

  return { openChannelCreate, createChannel, updateChannel }
}
