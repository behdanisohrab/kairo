import type { ReactNode } from 'react'

export function EmptyState({
  title,
  description,
  action,
  icon,
}: {
  title: string
  description?: string
  action?: ReactNode
  icon?: ReactNode
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-[18px] border border-dashed bg-[var(--color-raised)] px-6 py-10 text-center">
      {icon && (
        <div className="mb-3 inline-flex h-10 w-10 items-center justify-center rounded-2xl bg-white border text-[var(--color-ink-4)]">
          {icon}
        </div>
      )}
      <h4 className="text-sm font-semibold tracking-tight">{title}</h4>
      {description && <p className="mt-1 max-w-sm text-sm leading-5 text-[var(--color-ink-3)]">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
