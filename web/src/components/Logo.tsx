import { cn } from '@/lib/utils'

/** The melon slice: honeydew rind, cantaloupe flesh, watermelon heart. */
export function Logo({ className, withName = false }: { className?: string; withName?: boolean }) {
  return (
    <span className={cn('inline-flex items-center gap-2', className)}>
      <svg viewBox="0 0 48 48" className="size-[1em] shrink-0" aria-hidden="true">
        <g transform="rotate(-24 24 24)">
          <path d="M2 14a22 22 0 0 0 44 0z" fill="var(--honeydew)" />
          <path d="M7 14a17 17 0 0 0 34 0z" fill="var(--cantaloupe)" />
          <path d="M11 14a13 13 0 0 0 26 0z" fill="var(--watermelon)" />
          <g fill="#1D2418" opacity=".8">
            <ellipse cx="18" cy="19" rx="1.3" ry="2" />
            <ellipse cx="24" cy="22" rx="1.3" ry="2" />
            <ellipse cx="30" cy="19" rx="1.3" ry="2" />
          </g>
        </g>
      </svg>
      {withName && <span className="text-[0.8em] font-semibold tracking-tight">Melon</span>}
    </span>
  )
}
