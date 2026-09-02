import { describe, expect, it } from 'vitest'
import type { EventPage, RealtimeEvent } from '../services/chatApi'
import { syncRealtimeEvents } from './eventSync'

describe('syncRealtimeEvents', () => {
  it('continues past 100 pages until the event gap is fully recovered', async () => {
    const pages = new Map<number, EventPage>()
    for (let pageNumber = 0; pageNumber < 101; pageNumber += 1) {
      const sequence = pageNumber + 1
      pages.set(pageNumber, {
        events: [{ type: 'message.created', channel_id: 'general', event_id: sequence, sequence }],
        has_more: pageNumber < 100,
        next_cursor: pageNumber < 100 ? String(sequence) : undefined,
        cursor: sequence,
      })
    }

    const received: RealtimeEvent[] = []
    const requestedCursors: number[] = []
    let finalCursor = 0

    const recovered = await syncRealtimeEvents(
      0,
      async (cursor) => {
        requestedCursors.push(cursor)
        return pages.get(cursor) ?? { events: [], has_more: false, cursor }
      },
      (event) => { received.push(event) },
      (cursor) => { finalCursor = cursor },
    )

    expect(recovered).toBe(true)
    expect(requestedCursors).toHaveLength(101)
    expect(received).toHaveLength(101)
    expect(received[received.length - 1]?.sequence).toBe(101)
    expect(finalCursor).toBe(101)
  })

  it('stops when a paginated response does not advance the cursor', async () => {
    const received: RealtimeEvent[] = []
    const onCursor = () => { throw new Error('cursor should not advance') }

    await expect(syncRealtimeEvents(
      10,
      async () => ({
        events: [],
        has_more: true,
        next_cursor: '10',
        cursor: 10,
      }),
      (event) => { received.push(event) },
      onCursor,
    )).resolves.toBe(false)
    expect(received).toHaveLength(0)
  })
})
