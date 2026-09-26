import { describe, expect, it, vi } from 'vitest'
import type { RealtimeEvent } from '../services/chatApi'
import { clearScheduledReadTimers, shouldApplyLiveRealtimeEvent } from './useChatRealtime'

function event(sequence: number): RealtimeEvent {
  return { type: 'message.created', channel_id: 'visible-channel', sequence, event_id: sequence }
}

describe('shouldApplyLiveRealtimeEvent', () => {
  it('accepts a visible event after an inaccessible channel event', () => {
    expect(shouldApplyLiveRealtimeEvent(event(3), 1)).toBe(true)
  })

  it('rejects an event already covered by the current cursor', () => {
    expect(shouldApplyLiveRealtimeEvent(event(3), 3)).toBe(false)
  })

  it('accepts an AI completion that shares the persisted message sequence', () => {
    expect(shouldApplyLiveRealtimeEvent({ ...event(3), type: 'message.ai_completed' }, 3)).toBe(true)
  })

  it('accepts a delayed AI completion after a newer persisted event', () => {
    expect(shouldApplyLiveRealtimeEvent({ ...event(3), type: 'message.ai_completed' }, 4)).toBe(true)
  })
})

describe('clearScheduledReadTimers', () => {
  it('clears every pending channel read timer', () => {
    const clearTimer = vi.fn()

    clearScheduledReadTimers({ general: 11, frontend: 12 }, clearTimer)

    expect(clearTimer).toHaveBeenCalledWith(11)
    expect(clearTimer).toHaveBeenCalledWith(12)
    expect(clearTimer).toHaveBeenCalledTimes(2)
  })
})
