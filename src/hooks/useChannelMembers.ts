import { useCallback, useEffect, useRef, useState, type MutableRefObject } from 'react'
import { chatApi, type ApiChannelMember } from '../services/chatApi'

type UseChannelMembersOptions = {
  backendReady: boolean
  selectedChannelId: string
  selectedChannelRef: MutableRefObject<string>
}

export function useChannelMembers({ backendReady, selectedChannelId, selectedChannelRef }: UseChannelMembersOptions) {
  const [members, setMembers] = useState<ApiChannelMember[]>([])
  const [loaded, setLoaded] = useState(false)
  const requestSequenceRef = useRef(0)
  const abortControllerRef = useRef<AbortController | null>(null)

  const refresh = useCallback(async (channelId = selectedChannelRef.current, clearOnFailure = false) => {
    if (!backendReady) return
    const requestSequence = requestSequenceRef.current + 1
    requestSequenceRef.current = requestSequence
    abortControllerRef.current?.abort()
    const controller = new AbortController()
    abortControllerRef.current = controller

    try {
      const response = await chatApi.listChannelMembers(channelId, controller.signal)
      if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== channelId) return
      setMembers(response.members)
      setLoaded(true)
    } catch (error) {
      if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== channelId) return
      if (clearOnFailure) {
        setMembers([])
        setLoaded(true)
      }
      throw error
    } finally {
      if (requestSequence === requestSequenceRef.current) abortControllerRef.current = null
    }
  }, [backendReady, selectedChannelRef])

  useEffect(() => {
    if (!backendReady) return
    setLoaded(false)
    setMembers([])
    void refresh(selectedChannelId, true).catch(() => undefined)
    return () => abortControllerRef.current?.abort()
  }, [backendReady, refresh, selectedChannelId])

  useEffect(() => () => abortControllerRef.current?.abort(), [])

  return { members, loaded, refresh }
}
