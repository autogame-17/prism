import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Key, Plug, Shield, Waypoints } from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { useI18n } from '@/lib/i18n'

const KEY = 'prism.onboarded'

export function OnboardingDialog() {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const t = useI18n((s) => s.t)

  useEffect(() => {
    if (localStorage.getItem(KEY) !== 'true') {
      setOpen(true)
    }
  }, [])

  const finish = (gotoChannels: boolean) => {
    localStorage.setItem(KEY, 'true')
    setOpen(false)
    if (gotoChannels) navigate('/channels')
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && finish(false)}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('onboarding.title')}</DialogTitle>
          <DialogDescription>{t('onboarding.intro')}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <Step
            icon={<Shield className="h-4 w-4" />}
            title={t('onboarding.step.secret.title')}
            desc={t('onboarding.step.secret.desc')}
            done
          />
          <Step
            icon={<Plug className="h-4 w-4" />}
            title={t('onboarding.step.channel.title')}
            desc={t('onboarding.step.channel.desc')}
          />
          <Step
            icon={<Key className="h-4 w-4" />}
            title={t('onboarding.step.token.title')}
            desc={t('onboarding.step.token.desc')}
          />
          <Step
            icon={<Waypoints className="h-4 w-4" />}
            title={t('onboarding.step.tunnel.title')}
            desc={t('onboarding.step.tunnel.desc')}
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => finish(false)}>
            {t('onboarding.skip')}
          </Button>
          <Button onClick={() => finish(true)}>{t('onboarding.start')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Step({
  icon,
  title,
  desc,
  done,
}: {
  icon: React.ReactNode
  title: string
  desc: string
  done?: boolean
}) {
  return (
    <div className="flex gap-3 rounded-md border p-3">
      <div
        className={
          'mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full ' +
          (done ? 'bg-green-500/15 text-green-500' : 'bg-primary/15 text-primary')
        }
      >
        {icon}
      </div>
      <div>
        <p className="text-sm font-medium leading-tight">{title}</p>
        <p className="text-xs text-muted-foreground">{desc}</p>
      </div>
    </div>
  )
}
