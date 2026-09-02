import { describe, expect, it } from 'vitest'
import type { RealtimeEvent } from '../services/chatApi'
import { shouldApplyLiveRealtimeEvent } from './useChatRealtime'

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
})
