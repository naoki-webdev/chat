import { describe, expect, it } from 'vitest'
import { reduceMessageEvent } from './realtimeReducer'
import type { MessageMap } from '../types/messageState'
import type { RealtimeEvent } from '../services/chatApi'
import type { Message } from '../types/chat'

const baseEvent: RealtimeEvent = {
  type: 'message.created',
  channel_id: 'general',
  event_id: 1,
  sequence: 1,
  message: { id: 'm-1', channel_id: 'general', author_id: 'u-1', author: 'User', initials: 'U', color: '#fff', time: '10:00', body: 'hello' },
}

describe('reduceMessageEvent', () => {
  it('does not add an unloaded message for update-like events', () => {
    expect(reduceMessageEvent({}, { ...baseEvent, type: 'reaction.added' })).toEqual({})
  })

  it('applies AI deltas to an existing streaming message', () => {
    const current: MessageMap = { general: [{ id: 'ai-1', author: 'Orbit AI', initials: '✦', color: '#fff', time: '', body: 'Hel', streaming: true }] }
    const next = reduceMessageEvent(current, { ...baseEvent, type: 'message.ai_delta', message: undefined, message_id: 'ai-1', delta: 'lo' })

    expect(next.general[0]).toMatchObject({ id: 'ai-1', body: 'Hello', streaming: true })
  })

  it('removes a deleted reply and decrements its root count', () => {
    const root: Message = { id: 'm-1', authorID: 'u-1', author: 'User', initials: 'U', color: '#fff', time: '10:00', body: 'hello', threadCount: 2 }
    const reply: Message = { ...root, id: 'reply-1', parentMessageId: 'm-1', body: 'reply' }
    const current: MessageMap = { general: [root, reply] }
    const next = reduceMessageEvent(current, { ...baseEvent, type: 'message.deleted', message: undefined, message_id: 'reply-1', parent_message_id: 'm-1' })

    expect(next.general).toHaveLength(1)
    expect(next.general[0]?.threadCount).toBe(1)
  })
})
