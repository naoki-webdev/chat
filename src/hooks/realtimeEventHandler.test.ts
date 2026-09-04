import { describe, expect, it, vi } from 'vitest'
import type { Dispatch, MutableRefObject, SetStateAction } from 'react'
import type { RealtimeEvent } from '../services/chatApi'
import type { ApiUser } from '../services/chatApi'
import type { Channel, Message } from '../types/chat'
import { createRealtimeEventHandler, type TypingUsers } from './realtimeEventHandler'
import { type MessageMap } from '../types/messageState'

function ref<T>(current: T): MutableRefObject<T> {
  return { current }
}

function setter<T>(read: () => T, write: (value: T) => void): Dispatch<SetStateAction<T>> {
  return (update) => write(typeof update === 'function' ? (update as (value: T) => T)(read()) : update)
}

function event(overrides: Partial<RealtimeEvent> = {}): RealtimeEvent {
  return {
    type: 'message.created',
    channel_id: 'other-channel',
    event_id: 1,
    sequence: 1,
    message: {
      id: 'message-1',
      channel_id: 'other-channel',
      author_id: 'u-other',
      author: 'Other User',
      initials: 'OU',
      color: '#fff',
      time: '12:00',
      body: 'hello',
    },
    ...overrides,
  }
}

function createHarness(selectedChannel = 'current-channel') {
  let channels: Channel[] = [{ id: 'other-channel', name: 'other', group: 'Engineering', kind: 'channel', unread: 0 }]
  let messages: MessageMap = {}
  let typingUsers: TypingUsers = {}
  let myPresence: NonNullable<Channel['presence']> = 'online'
  let threadRoot: Message | null = null
  let threadReplies: Message[] = []
  const refreshChannels = vi.fn(async () => undefined)
  const refreshSelectedChannelMembers = vi.fn(async () => undefined)
  const addThreadReply = vi.fn()
  const scheduleSelectedChannelRead = vi.fn()
  const handler = createRealtimeEventHandler({
    currentUserID: 'u-me',
    setAuthUser: vi.fn() as Dispatch<SetStateAction<ApiUser | null>>,
    eventCursorRef: ref(0),
    selectedChannelRef: ref(selectedChannel),
    threadRootRef: ref(null),
    refreshChannelsRef: ref(refreshChannels),
    refreshSelectedChannelMembersRef: ref(refreshSelectedChannelMembers),
    scheduleSelectedChannelReadRef: ref(scheduleSelectedChannelRead),
    setChannels: setter(() => channels, (value) => { channels = value }),
    setMessages: setter(() => messages, (value) => { messages = value }),
    setTypingUsers: setter(() => typingUsers, (value) => { typingUsers = value }),
    setMyPresence: setter(() => myPresence, (value) => { myPresence = value }),
    setThreadRoot: setter(() => threadRoot, (value) => { threadRoot = value }),
    setThreadReplies: setter(() => threadReplies, (value) => { threadReplies = value }),
    advanceEventCursor: vi.fn(),
    addThreadReply,
  })
  return { handler, addThreadReply, scheduleSelectedChannelRead, refreshChannels, refreshSelectedChannelMembers, getChannels: () => channels, getMessages: () => messages }
}

describe('createRealtimeEventHandler', () => {
  it('counts a new reply as unread even when the thread is not open', () => {
    const harness = createHarness()

    harness.handler(event({
      message: {
        ...event().message!,
        id: 'reply-1',
        parent_message_id: 'root-1',
      },
    }))

    expect(harness.addThreadReply).toHaveBeenCalledWith(expect.objectContaining({ id: 'reply-1' }), 'other-channel', true)
    expect(harness.getChannels()[0]?.unread).toBe(1)
  })

  it('does not count the current user\'s own message as unread', () => {
    const harness = createHarness()

    harness.handler(event({
      message: {
        ...event().message!,
        author_id: 'u-me',
      },
    }))

    expect(harness.getChannels()[0]?.unread).toBe(0)
  })

  it('does not append an unloaded message for an update or reaction event', () => {
    const harness = createHarness()

    harness.handler(event({ type: 'reaction.added' }))

    expect(harness.getMessages()).toEqual({})
    expect(harness.getChannels()[0]?.unread).toBe(0)
  })

  it('schedules the server read cursor for a new message in the selected channel', () => {
    const harness = createHarness('current-channel')

    harness.handler(event({
      channel_id: 'current-channel',
      message: {
        ...event().message!,
        channel_id: 'current-channel',
      },
    }))

    expect(harness.scheduleSelectedChannelRead).toHaveBeenCalledWith('current-channel')
  })

  it('does not request members again after the current user is removed', async () => {
    const harness = createHarness('current-channel')

    await harness.handler(event({
      type: 'channel.member_removed',
      channel_id: 'current-channel',
      message: undefined,
      member_id: 'u-me',
    }))

    expect(harness.refreshChannels).toHaveBeenCalledOnce()
    expect(harness.refreshSelectedChannelMembers).not.toHaveBeenCalled()
  })

  it('updates a renamed user DM using the stable previous handle', () => {
    const harness = createHarness()
    harness.getChannels().push({ id: 'old-handle', name: 'Old Name', group: 'Direct messages', kind: 'dm', unread: 0 })

    harness.handler(event({
      type: 'user.updated',
      message: undefined,
      actor_id: 'u-other',
      actor_name: 'New Name',
      actor_handle: 'new-handle',
      previous_actor_handle: 'old-handle',
      actor_initials: 'NN',
      actor_color: '#000',
    }))

    expect(harness.getChannels().find((channel) => channel.id === 'old-handle')).toMatchObject({ name: 'New Name', initials: 'NN', color: '#000' })
  })

  it('updates a renamed user DM using the stable user ID', () => {
    const harness = createHarness()
    harness.getChannels().push({ id: 'old-channel-id', name: 'Old Name', group: 'Direct messages', kind: 'dm', unread: 0, peerUserID: 'u-other' })

    harness.handler(event({
      type: 'user.updated',
      message: undefined,
      actor_id: 'u-other',
      actor_name: 'New Name',
      actor_handle: 'new-handle',
      previous_actor_handle: 'old-handle',
      actor_initials: 'NN',
      actor_color: '#000',
    }))

    expect(harness.getChannels().find((channel) => channel.id === 'old-channel-id')).toMatchObject({ name: 'New Name', initials: 'NN', color: '#000' })
  })
})
