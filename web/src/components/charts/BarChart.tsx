// Minimal dependency-free SVG bar chart for the Traffic page. Renders one
// bar per bucket; gaps in the data simply show as missing bars.
import { useMemo } from 'react'
import type { TrafficBucket } from '../../api'

interface Props {
  buckets: TrafficBucket[]
  height?: number
  label?: string
}

export default function BarChart({ buckets, height = 160, label }: Props) {
  const max = useMemo(() => Math.max(1, ...buckets.map((b) => b.count)), [buckets])

  if (buckets.length === 0) {
    return <p className="p-6 text-center text-sm text-[var(--color-ink-3)]">{label}</p>
  }

  return (
    <div className="w-full overflow-x-auto">
      <svg
        viewBox={`0 0 ${buckets.length * 14} ${height + 18}`}
        preserveAspectRatio="xMinYMax meet"
        className="w-full"
        style={{ minWidth: Math.max(280, buckets.length * 14) }}
        role="img"
        aria-label={label}
      >
        {buckets.map((b, i) => {
          const h = Math.max(2, Math.round((b.count / max) * (height - 20)))
          const x = i * 14 + 3
          return (
            <g key={b.bucket}>
              <title>{`${b.bucket.replace('T', ' ').replace(':00:00Z', '')}:00 UTC — ${b.count}`}</title>
              <rect x={x} y={height - h} width={8} height={h} rx={2}
                fill="var(--color-brand)" opacity={b.count === max ? 1 : 0.55} />
              {(buckets.length <= 26 || i % Math.ceil(buckets.length / 13) === 0) && (
                <text x={x + 4} y={height + 12} textAnchor="middle" fontSize={7}
                  fill="var(--color-ink-4)" className="mono">
                  {b.bucket.slice(5, 10).replace('T', ' ')}
                </text>
              )}
            </g>
          )
        })}
      </svg>
    </div>
  )
}
