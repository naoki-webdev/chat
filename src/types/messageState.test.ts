import { describe, expect, it } from 'vitest'
import { replaceMessageInMap, type MessageMap, updateMessagesByAuthor, upsertMessageInMap } from './messageState'
import type { Message } from './chat'

const message: Message = {
  id: 'm-1',
  authorID: 'u-1',
  author: 'Old Name',
  initials: 'ON',
  color: '#fff',
  time: '10:00',
  body: 'hello',
}

describe('message state helpers', () => {
  it('merges an existing message without losing local reaction state', () => {
    const current: MessageMap = { general: [{ ...message, reactions: [{ emoji: '👍', count: 1, reacted: true }] }] }
    const next = upsertMessageInMap(current, 'general', { ...message, reactions: [{ emoji: '👍', count: 2 }] })

    expect(next.general[0]).toMatchObject({ id: 'm-1', reactions: [{ emoji: '👍', count: 2, reacted: true }] })
  })

  it('does not append an unloaded message for a non-create event', () => {
    expect(upsertMessageInMap({}, 'general', message, false)).toEqual({})
  })

  it('replaces a loaded message without appending an unloaded one', () => {
    const current: MessageMap = { general: [message] }

    expect(replaceMessageInMap(current, 'general', { ...message, body: 'deleted' }).general[0].body).toBe('deleted')
    expect(replaceMessageInMap(current, 'general', { ...message, id: 'm-2' })).toBe(current)
  })

  it('updates the author presentation across loaded channels', () => {
    const next = updateMessagesByAuthor({ general: [message] }, 'u-1', { author: 'New Name', initials: 'NN', color: '#000' })

    expect(next.general[0]).toMatchObject({ author: 'New Name', initials: 'NN', color: '#000' })
  })
})
