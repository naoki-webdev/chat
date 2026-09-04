import { useCallback, type MutableRefObject } from 'react'
import type { EventPage, RealtimeEvent } from '../services/chatApi'
import { syncRealtimeEvents } from './eventSync'
import { enqueueRealtimeTask, type RealtimeQueueRef } from './realtimeQueue'

type UseRealtimeSyncOptions = {
  realtimeQueueRef: RealtimeQueueRef
  eventCursorRef: MutableRefObject<number>
  selectedChannelRef: MutableRefObject<string>
  loadMessagesDirectRef: MutableRefObject<(channelId: string) => Promise<void>>
  refreshChannelsWithRetryRef: MutableRefObject<() => Promise<void>>
  refreshSelectedChannelMembersRef: MutableRefObject<() => Promise<void>>
  listEvents: (after: number) => Promise<EventPage>
  onEvent: (event: RealtimeEvent) => void | Promise<void>
  onCursor: (cursor: number) => void
  requestReconnect: () => void
}

export function useRealtimeSync({
  realtimeQueueRef,
  eventCursorRef,
  selectedChannelRef,
  loadMessagesDirectRef,
  refreshChannelsWithRetryRef,
  refreshSelectedChannelMembersRef,
  listEvents,
  onEvent,
  onCursor,
  requestReconnect,
}: UseRealtimeSyncOptions) {
  const syncEvents = useCallback(async (after: number) => {
    return syncRealtimeEvents(after, listEvents, onEvent, onCursor)
  }, [listEvents, onCursor, onEvent])

  const enqueueEventSync = useCallback(() => {
    void enqueueRealtimeTask(realtimeQueueRef, async () => {
      if (!await syncEvents(eventCursorRef.current)) throw new Error('realtime event sync did not converge')
      await refreshChannelsWithRetryRef.current()
      await refreshSelectedChannelMembersRef.current()
      await loadMessagesDirectRef.current(selectedChannelRef.current)
    }).catch(() => requestReconnect())
  }, [eventCursorRef, loadMessagesDirectRef, realtimeQueueRef, refreshChannelsWithRetryRef, refreshSelectedChannelMembersRef, requestReconnect, selectedChannelRef, syncEvents])

  return { enqueueEventSync }
}
