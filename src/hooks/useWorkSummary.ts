import { useEffect, useRef, useState, type MutableRefObject } from 'react'
import { chatApi, type ApiChannelSummary } from '../services/chatApi'
import { t } from '../i18n'

type UseWorkSummaryOptions = {
  backendReady: boolean
  backendUnavailableMessage: string
  selectedChannelId: string
  selectedChannelRef: MutableRefObject<string>
}

export function useWorkSummary({ backendReady, backendUnavailableMessage, selectedChannelId, selectedChannelRef }: UseWorkSummaryOptions) {
  const [workSummary, setWorkSummary] = useState<ApiChannelSummary | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [summaryError, setSummaryError] = useState<string | null>(null)
  const requestSequenceRef = useRef(0)
  const abortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    requestSequenceRef.current += 1
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
    setSummaryLoading(false)
    setWorkSummary(null)
    setSummaryError(null)
  }, [selectedChannelId])

  useEffect(() => () => {
    requestSequenceRef.current += 1
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
  }, [])

  const generateSummary = async () => {
    if (!backendReady) {
      setSummaryError(backendUnavailableMessage)
      return
    }
    const requestChannelId = selectedChannelId
    const requestSequence = requestSequenceRef.current + 1
    requestSequenceRef.current = requestSequence
    abortControllerRef.current?.abort()
    const controller = new AbortController()
    abortControllerRef.current = controller
    setSummaryLoading(true)
    setSummaryError(null)
    try {
      const summary = await chatApi.summarizeChannel(requestChannelId, controller.signal)
      if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId) return
      setWorkSummary(summary)
    } catch {
      if (controller.signal.aborted || requestSequence !== requestSequenceRef.current || selectedChannelRef.current !== requestChannelId) return
      setSummaryError(t('errors.summary'))
    } finally {
      if (requestSequence === requestSequenceRef.current) {
        abortControllerRef.current = null
        setSummaryLoading(false)
      }
    }
  }

  return { workSummary, summaryLoading, summaryError, setSummaryError, generateSummary }
}
