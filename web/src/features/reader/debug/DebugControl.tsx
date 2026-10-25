import { Bug } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export function DebugControl({ enabled, pending, onToggle }: { enabled: boolean; pending: boolean; onToggle: (on: boolean) => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex items-center gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className={cn('relative', enabled && 'text-gold-text hover:text-gold-text')}
            disabled={pending}
            aria-label={t('debug.toggle')}
            aria-pressed={enabled}
            onClick={() => onToggle(!enabled)}
          >
            <Bug />
            {enabled && <span aria-hidden className="absolute top-1.5 right-1.5 size-1 rounded-full bg-gold" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t(enabled ? 'debug.pauseCapture' : 'debug.startCapture')}</TooltipContent>
      </Tooltip>
    </div>
  )
}
