import { useCallback, useEffect, useState } from 'react'
import {
  onEvent,
  tunnelRotate,
  tunnelStart,
  tunnelStatus,
  tunnelStop,
  type TunnelSnapshot,
} from '@/lib/wails'

export function useTunnel() {
  const [snap, setSnap] = useState<TunnelSnapshot | null>(null)
  const [busy, setBusy] = useState(false)

  const refresh = useCallback(async () => {
    const s = await tunnelStatus()
    setSnap(s)
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, 3000)
    const offUrlPromise = onEvent<string>('tunnel.url', () => refresh())
    const offStatusPromise = onEvent<TunnelSnapshot>('tunnel.status', (s) => setSnap(s))
    return () => {
      clearInterval(id)
      offUrlPromise.then((off) => off())
      offStatusPromise.then((off) => off())
    }
  }, [refresh])

  const start = useCallback(async () => {
    setBusy(true)
    try {
      await tunnelStart()
    } finally {
      await refresh()
      setBusy(false)
    }
  }, [refresh])

  const stop = useCallback(async () => {
    setBusy(true)
    try {
      await tunnelStop()
    } finally {
      await refresh()
      setBusy(false)
    }
  }, [refresh])

  const rotate = useCallback(async () => {
    setBusy(true)
    try {
      await tunnelRotate()
    } finally {
      await refresh()
      setBusy(false)
    }
  }, [refresh])

  return { snap, busy, start, stop, rotate, refresh }
}
