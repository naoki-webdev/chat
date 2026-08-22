import { useEffect, useState } from 'react'
import type { SavedMessageRef } from '../components/WorkspaceOverlay'

const storageKey = (userID: string) => `orbit:saved-message-refs:${userID}`

export function useSavedMessages(userID: string | null) {
  const [savedMessages, setSavedMessages] = useState<SavedMessageRef[]>([])
  const [loadedFor, setLoadedFor] = useState<string | null>(null)

  useEffect(() => {
    setLoadedFor(null)
    if (!userID) {
      setSavedMessages([])
      return
    }
    try {
      const stored = window.localStorage.getItem(storageKey(userID))
      setSavedMessages(stored ? JSON.parse(stored) as SavedMessageRef[] : [])
    } catch {
      setSavedMessages([])
    }
    setLoadedFor(userID)
  }, [userID])

  useEffect(() => {
    if (!userID || loadedFor !== userID) return
    window.localStorage.setItem(storageKey(userID), JSON.stringify(savedMessages))
  }, [loadedFor, savedMessages, userID])

  return [savedMessages, setSavedMessages] as const
}
