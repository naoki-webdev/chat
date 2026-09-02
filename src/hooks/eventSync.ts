import type { EventPage, RealtimeEvent } from '../services/chatApi'

type ListEvents = (after: number) => Promise<EventPage>
type EventHandler = (event: RealtimeEvent) => void | Promise<void>

export async function syncRealtimeEvents(
  after: number,
  listEvents: ListEvents,
  onEvent: EventHandler,
  onCursor: (cursor: number) => void,
): Promise<boolean> {
  let cursor = after

  while (true) {
    const page = await listEvents(cursor)
    for (const event of page.events) {
      await onEvent(event)
    }

    if (!page.has_more) {
      onCursor(page.cursor)
      return true
    }

    const nextCursor = Number(page.next_cursor)
    if (!Number.isFinite(nextCursor) || nextCursor <= cursor) return false
    cursor = nextCursor
  }
}
