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
  isSessionActive?: () => boolean
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
  isSessionActive,
}: UseRealtimeSyncOptions) {
  const sessionIsActive = useCallback(() => isSessionActive?.() ?? true, [isSessionActive])

  const syncEvents = useCallback(async (after: number) => {
    return syncRealtimeEvents(
      after,
      listEvents,
      (event) => {
        if (!sessionIsActive()) return
        return onEvent(event)
      },
      (cursor) => {
        if (sessionIsActive()) onCursor(cursor)
      },
    )
  }, [listEvents, onCursor, onEvent, sessionIsActive])

  const enqueueEventSync = useCallback(() => {
    void enqueueRealtimeTask(realtimeQueueRef, async () => {
      if (!sessionIsActive()) return
      if (!await syncEvents(eventCursorRef.current)) throw new Error('realtime event sync did not converge')
      if (!sessionIsActive()) return
      await refreshChannelsWithRetryRef.current()
      if (!sessionIsActive()) return
      await refreshSelectedChannelMembersRef.current()
      if (!sessionIsActive()) return
      await loadMessagesDirectRef.current(selectedChannelRef.current)
    }).catch(() => {
      if (sessionIsActive()) requestReconnect()
    })
  }, [eventCursorRef, loadMessagesDirectRef, realtimeQueueRef, refreshChannelsWithRetryRef, refreshSelectedChannelMembersRef, requestReconnect, selectedChannelRef, sessionIsActive, syncEvents])

  return { enqueueEventSync }
}
