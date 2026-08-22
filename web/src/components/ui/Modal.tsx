import { useEffect, useRef, type ReactNode } from 'react'

export function Modal({
  open,
  onClose,
  title,
  description,
  children,
}: {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: ReactNode
}) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    setTimeout(() => ref.current?.querySelector<HTMLElement>('button,input,select,textarea')?.focus(), 10)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [open, onClose])

  if (!open) return null
  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center p-4">
      <button
        aria-label="Close"
        onClick={onClose}
        className="absolute inset-0 bg-[#0f0e0d]/60 backdrop-blur-[8px]"
      />
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        className="animate-scale relative w-full max-w-lg rounded-[18px] border bg-[var(--color-surface)] p-6 shadow-medium"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="mb-4">
          <h3 id="modal-title" className="text-base font-semibold tracking-tight text-[var(--color-ink)]">
            {title}
          </h3>
          {description && <p className="mt-1 text-sm leading-5 text-[var(--color-ink-3)]">{description}</p>}
        </div>
        {children}
      </div>
    </div>
  )
}

export function ConfirmModal({
  open,
  onClose,
  onConfirm,
  title,
  description,
  confirmText = 'Confirm',
  variant = 'danger',
  loading = false,
}: {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  title: string
  description?: string
  confirmText?: string
  variant?: 'danger' | 'primary'
  loading?: boolean
}) {
  return (
    <Modal open={open} onClose={onClose} title={title} description={description}>
      <div className="mt-6 flex justify-end gap-2">
        <button onClick={onClose} className="btn btn-ghost" disabled={loading}>
          Cancel
        </button>
        <button
          onClick={onConfirm}
          disabled={loading}
          className={`btn ${variant === 'danger' ? 'btn-danger' : 'btn-primary'}`}
        >
          {loading ? 'Please wait…' : confirmText}
        </button>
      </div>
    </Modal>
  )
}
