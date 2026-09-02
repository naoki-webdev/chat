import type { MutableRefObject } from 'react'

export type RealtimeQueueRef = MutableRefObject<Promise<void>>

export function enqueueRealtimeTask<T>(queueRef: RealtimeQueueRef, task: () => T | Promise<T>): Promise<T> {
  const result = queueRef.current.then(task)
  queueRef.current = result.then(() => undefined, () => undefined)
  return result
}
