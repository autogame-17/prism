import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import {
  onEvent,
  tunnelRotate,
  tunnelStart,
  tunnelStatus,
  tunnelStop,
  type TunnelSnapshot,
} from '@/lib/wails'

// tunnelStart / tunnelStop / tunnelRotate are strict() wrappers: they throw
// when the underlying Wails call fails. Surface those failures with a toast
// so the user isn't staring at a stale "stopped" badge wondering why the
// click did nothing. tunnel.snap.lastError covers backend-reported errors,
// but the wails IPC layer itself (e.g. binding panic, encoding) only shows
// up here.
function reportTunnelError(action: string, err: unknown) {
  const msg = err instanceof Error ? err.message : String(err)
  toast.error(`${action}: ${msg}`)
}

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
    } catch (err) {
      reportTunnelError('tunnel start failed', err)
    } finally {
      await refresh()
      setBusy(false)
    }
  }, [refresh])

  const stop = useCallback(async () => {
    setBusy(true)
    try {
      await tunnelStop()
    } catch (err) {
      reportTunnelError('tunnel stop failed', err)
    } finally {
      await refresh()
      setBusy(false)
    }
  }, [refresh])

  const rotate = useCallback(async () => {
    setBusy(true)
    try {
      await tunnelRotate()
    } catch (err) {
      reportTunnelError('tunnel rotate failed', err)
    } finally {
      await refresh()
      setBusy(false)
    }
  }, [refresh])

  return { snap, busy, start, stop, rotate, refresh }
}
