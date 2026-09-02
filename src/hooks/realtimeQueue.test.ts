import { describe, expect, it } from 'vitest'
import { enqueueRealtimeTask, type RealtimeQueueRef } from './realtimeQueue'

describe('enqueueRealtimeTask', () => {
  it('runs tasks in enqueue order', async () => {
    const queueRef = { current: Promise.resolve() } as RealtimeQueueRef
    const order: string[] = []
    let releaseFirst!: () => void

    const first = enqueueRealtimeTask(queueRef, async () => {
      order.push('first:start')
      await new Promise<void>((resolve) => { releaseFirst = resolve })
      order.push('first:end')
    })
    const second = enqueueRealtimeTask(queueRef, async () => { order.push('second') })

    await Promise.resolve()
    expect(order).toEqual(['first:start'])
    releaseFirst()
    await Promise.all([first, second])
    expect(order).toEqual(['first:start', 'first:end', 'second'])
  })

  it('continues with the next task after a failure', async () => {
    const queueRef = { current: Promise.resolve() } as RealtimeQueueRef
    const order: string[] = []

    await expect(enqueueRealtimeTask(queueRef, async () => {
      order.push('failed')
      throw new Error('expected')
    })).rejects.toThrow('expected')
    await enqueueRealtimeTask(queueRef, async () => { order.push('next') })

    expect(order).toEqual(['failed', 'next'])
  })
})
