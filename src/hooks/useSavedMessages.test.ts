import { describe, expect, it } from 'vitest'
import { parseSavedMessages } from './useSavedMessages'

describe('parseSavedMessages', () => {
  it('discards invalid JSON, non-array values, and malformed references', () => {
    expect(parseSavedMessages('{')).toEqual([])
    expect(parseSavedMessages(JSON.stringify({ channelId: 'general', messageId: 'message-1' }))).toEqual([])
    expect(parseSavedMessages(JSON.stringify([
      null,
      'bad',
      { channelId: '', messageId: 'message-1' },
      { channelId: 'general', messageId: 1 },
      { channelId: 'general', messageId: 'message-1' },
      { channelId: 'general', messageId: 'message-1', extra: true },
    ]))).toEqual([{ channelId: 'general', messageId: 'message-1' }])
  })
})
