export function Skeleton({ className = '', style }: { className?: string; style?: React.CSSProperties }) {
  return <div className={`skeleton ${className}`} style={style} aria-hidden />
}

export function CardSkeleton() {
  return (
    <div className="card p-5">
      <div className="skeleton h-4 w-28 mb-3" />
      <div className="skeleton h-8 w-20 mb-2" />
      <div className="skeleton h-3 w-36" />
    </div>
  )
}

export function TableSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="card p-0 overflow-hidden">
      <div className="p-4 border-b" style={{ borderColor: 'var(--color-border)' }}>
        <div className="skeleton h-9 w-full" />
      </div>
      <div className="p-3 space-y-3">
        {Array.from({ length: rows }).map((_, i) => (
          <div key={i} className="flex gap-3">
            <div className="skeleton h-4 flex-1" />
            <div className="skeleton h-4 w-24" />
            <div className="skeleton h-4 w-32" />
          </div>
        ))}
      </div>
    </div>
  )
}
