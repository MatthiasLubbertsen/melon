import { useState, type ReactNode } from 'react'
import { Check, Copy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

type Tone = 'honeydew' | 'cantaloupe' | 'watermelon'

const tones: Record<Tone, string> = {
  honeydew: 'bg-honeydew-tint text-honeydew-ink',
  cantaloupe: 'bg-cantaloupe-tint text-cantaloupe-ink',
  watermelon: 'bg-watermelon-tint text-watermelon-ink',
}

/** A quiet, tinted label. */
export function Chip({ tone, children, className }: { tone: Tone; children: ReactNode; className?: string }) {
  return (
    <span className={cn('inline-flex h-6 shrink-0 items-center gap-1 rounded-full px-2.5 text-xs font-medium whitespace-nowrap [&>svg]:size-3.5', tones[tone], className)}>
      {children}
    </span>
  )
}

/** A tinted banner for things the user should not miss. */
export function Banner({ tone, icon, title, children, action, below }: { tone: Tone; icon?: ReactNode; title: ReactNode; children?: ReactNode; action?: ReactNode; below?: ReactNode }) {
  return (
    <div role="status" className={cn('rounded-xl px-3.5 py-3 text-sm', tones[tone])}>
      <div className="flex items-start gap-3 [&>svg]:mt-0.5 [&>svg]:size-4 [&>svg]:shrink-0">
        {icon}
        <div className="min-w-0 flex-1">
          <p className="font-medium">{title}</p>
          {children && <div className="mt-0.5 opacity-90">{children}</div>}
        </div>
        {action}
      </div>
      {/* Full width, flush with the banner's left edge. */}
      {below && <div className="mt-1.5 opacity-90">{below}</div>}
    </div>
  )
}

export function Avatar({ name, className }: { name: string; className?: string }) {
  const parts = name.trim().split(/\s+/)
  const initials = ((parts[0]?.[0] ?? '') + (parts.length > 1 ? (parts[parts.length - 1][0] ?? '') : '')).toUpperCase()
  return (
    <span aria-hidden="true" className={cn('inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-honeydew-tint text-xs font-semibold text-honeydew-ink', className)}>
      {initials || '?'}
    </span>
  )
}

/** A read-only value with a copy button. */
export function CopyField({ value, mono = true }: { value: string; mono?: boolean }) {
  const { t } = useI18n()
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    } catch {
      /* clipboard unavailable: the field is selectable */
    }
  }
  return (
    <div className="flex items-center gap-2">
      <input
        readOnly
        value={value}
        onFocus={(e) => e.currentTarget.select()}
        className={cn('h-11 min-w-0 flex-1 rounded-lg border bg-muted px-3 text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50', mono && 'font-mono text-xs')}
      />
      <Button type="button" variant="outline" className="h-11 shrink-0 px-3" onClick={copy} aria-label={t('copy')}>
        {copied ? <Check className="text-honeydew-ink" /> : <Copy />}
        <span className="hidden sm:inline">{copied ? t('copied') : t('copy')}</span>
      </Button>
    </div>
  )
}

/** Confirmation for destructive actions. Always watermelon. */
export function ConfirmDialog({
  open, onOpenChange, title, text, confirmLabel, onConfirm, busy,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  text: string
  confirmLabel: string
  onConfirm: () => void
  busy?: boolean
}) {
  const { t } = useI18n()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{text}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" className="h-11" onClick={() => onOpenChange(false)}>
            {t('cancel')}
          </Button>
          <Button variant="destructive" className="h-11" disabled={busy} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
