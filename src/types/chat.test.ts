import { describe, expect, it } from 'vitest'
import { mergeMessage, type Message } from './chat'

const message: Message = {
  id: 'message-1',
  author: 'Taro Tanaka',
  initials: 'TT',
  color: '#c56cf0',
  time: '10:00',
  body: '確認しました',
  reactions: [{ emoji: '👍', count: 1, reacted: true }],
}

describe('mergeMessage', () => {
  it('keeps the local reaction state when a broadcast omits it', () => {
    const incoming = { ...message, reactions: [{ emoji: '👍', count: 2 }] }

    expect(mergeMessage(message, incoming).reactions).toEqual([{ emoji: '👍', count: 2, reacted: true }])
  })

  it('accepts an explicit reaction state from an API response', () => {
    const incoming = { ...message, reactions: [{ emoji: '👍', count: 1, reacted: false }] }

    expect(mergeMessage(message, incoming).reactions).toEqual([{ emoji: '👍', count: 1, reacted: false }])
  })
})
