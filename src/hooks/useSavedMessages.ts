import { useEffect, useState } from 'react'
import type { SavedMessageRef } from '../components/WorkspaceOverlay'

const storageKey = (userID: string) => `orbit:saved-message-refs:${userID}`

export function parseSavedMessages(value: string | null): SavedMessageRef[] {
  if (!value) return []

  try {
    const parsed: unknown = JSON.parse(value)
    if (!Array.isArray(parsed)) return []

    const seen = new Set<string>()
    return parsed.flatMap((item): SavedMessageRef[] => {
      if (!item || typeof item !== 'object') return []
      const reference = item as Record<string, unknown>
      if (typeof reference.channelId !== 'string' || !reference.channelId.trim()) return []
      if (typeof reference.messageId !== 'string' || !reference.messageId.trim()) return []
      const key = `${reference.channelId}:${reference.messageId}`
      if (seen.has(key)) return []
      seen.add(key)
      return [{ channelId: reference.channelId, messageId: reference.messageId }]
    })
  } catch {
    return []
  }
}

export function useSavedMessages(userID: string | null) {
  const [savedMessages, setSavedMessages] = useState<SavedMessageRef[]>([])
  const [loadedFor, setLoadedFor] = useState<string | null>(null)

  useEffect(() => {
    setLoadedFor(null)
    setSavedMessages([])
    if (!userID) {
      return
    }
    try {
      const stored = window.localStorage.getItem(storageKey(userID))
      setSavedMessages(parseSavedMessages(stored))
    } catch {
      setSavedMessages([])
    }
    setLoadedFor(userID)
  }, [userID])

  useEffect(() => {
    if (!userID || loadedFor !== userID) return
    try {
      window.localStorage.setItem(storageKey(userID), JSON.stringify(savedMessages))
    } catch {
      return
    }
  }, [loadedFor, savedMessages, userID])

  return [savedMessages, setSavedMessages] as const
}
