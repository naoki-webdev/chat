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
  sessionVersion: number
  isSessionVersionActive: (version: number) => boolean
  selectChannel: (channel: Channel) => void
  setChannels: Dispatch<SetStateAction<Channel[]>>
  setMessages: Dispatch<SetStateAction<Record<string, Message[]>>>
  refreshSelectedChannelMembers: (channelId?: string) => Promise<void>
  setChannelCreateGroup: Dispatch<SetStateAction<string | null>>
  setChannelEditOpen: Dispatch<SetStateAction<boolean>>
  setActionError: Dispatch<SetStateAction<string | null>>
}

export function useChannelManagement({
  backendReady,
  backendUnavailableMessage,
  selectedChannelId,
  sessionVersion,
  isSessionVersionActive,
  selectChannel,
  setChannels,
  setMessages,
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
    const requestSessionVersion = sessionVersion
    try {
      const channel = fromApiChannel(await chatApi.createChannel({ name: payload.name, group: payload.group, kind: 'channel', description: payload.description, member_ids: payload.memberIds }))
      if (!isSessionVersionActive(requestSessionVersion)) return
      setChannels((current) => current.some((item) => item.id === channel.id) ? current : [...current, channel])
      setMessages((current) => current[channel.id] ? current : { ...current, [channel.id]: [] })
      selectChannel(channel)
      setChannelCreateGroup(null)
      setActionError(null)
    } catch (error) {
      if (!isSessionVersionActive(requestSessionVersion)) return
      setActionError(error instanceof ChatApiError && error.status === 409 ? t('errors.channelConflict') : t('errors.channelCreate'))
      throw Object.assign(new Error('channel creation failed'), { cause: error })
    }
  }, [backendReady, backendUnavailableMessage, isSessionVersionActive, selectChannel, sessionVersion, setActionError, setChannelCreateGroup, setChannels, setMessages])

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
        // チャンネル更新はすでに成功しているため、次回更新までは現在のメンバー表示を維持します。
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
