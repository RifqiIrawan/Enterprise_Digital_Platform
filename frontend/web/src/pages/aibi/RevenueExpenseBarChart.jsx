import { useMemo, useState } from 'react'

const WIDTH = 560
const HEIGHT = 220
const PAD_LEFT = 8
const PAD_RIGHT = 8
const PAD_TOP = 20
const PAD_BOTTOM = 28
const BAR_MAX_THICKNESS = 24
const BAR_GAP = 2

// Grouped bar chart: dua seri (revenue, expense) per bulan, sumbu tunggal
// (satuan sama, mata uang) -- warna kategorikal urutan tetap (slot 1 biru =
// Revenue, slot 2 oranye = Expense, lihat dataviz skill palette.md), bukan
// warna status (bg-success/bg-danger) supaya tidak dikira "baik/buruk".
// Tooltip satu per grup bulan (bukan per bar) supaya kedua nilai selalu
// terbaca bersamaan tanpa perlu hover dua kali.
const SERIES = [
  { key: 'revenue', label: 'Revenue', color: 'var(--bs-primary)' },
  { key: 'expense', label: 'Expense', color: 'var(--bs-orange)' },
]

function RevenueExpenseBarChart({ data, formatValue }) {
  const [hoverIndex, setHoverIndex] = useState(null)

  const maxValue = useMemo(() => {
    const values = data.flatMap((d) => [d.revenue, d.expense])
    return Math.max(1, ...values)
  }, [data])

  if (data.length === 0) {
    return <div className="text-secondary small">Belum ada data.</div>
  }

  const plotWidth = WIDTH - PAD_LEFT - PAD_RIGHT
  const plotHeight = HEIGHT - PAD_TOP - PAD_BOTTOM
  const groupWidth = plotWidth / data.length
  const barThickness = Math.min(BAR_MAX_THICKNESS, (groupWidth - BAR_GAP * 3) / 2)

  const yForValue = (v) => PAD_TOP + plotHeight - (v / maxValue) * plotHeight
  const gridSteps = [0, 0.25, 0.5, 0.75, 1]

  const hovered = hoverIndex != null ? data[hoverIndex] : null

  return (
    <div className="position-relative">
      <div className="d-flex gap-3 small text-secondary mb-1">
        {SERIES.map((s) => (
          <span key={s.key} className="d-flex align-items-center gap-1">
            <span style={{ width: 10, height: 10, borderRadius: 2, background: s.color, display: 'inline-block' }} />
            {s.label}
          </span>
        ))}
      </div>

      <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} style={{ width: '100%', height: 'auto', overflow: 'visible' }}>
        {gridSteps.map((step) => (
          <line
            key={step}
            x1={PAD_LEFT}
            y1={PAD_TOP + plotHeight * (1 - step)}
            x2={WIDTH - PAD_RIGHT}
            y2={PAD_TOP + plotHeight * (1 - step)}
            stroke="currentColor"
            strokeOpacity="0.1"
            strokeWidth="1"
          />
        ))}

        {data.map((d, i) => {
          const groupX = PAD_LEFT + i * groupWidth
          const groupCenter = groupX + groupWidth / 2
          const isHovered = i === hoverIndex
          return (
            <g key={d.month} opacity={hoverIndex != null && !isHovered ? 0.5 : 1}>
              {SERIES.map((s, si) => {
                const value = d[s.key]
                const barX = groupCenter - barThickness - BAR_GAP / 2 + si * (barThickness + BAR_GAP)
                const barY = yForValue(value)
                const barHeight = PAD_TOP + plotHeight - barY
                return (
                  <rect
                    key={s.key}
                    x={barX}
                    y={barY}
                    width={barThickness}
                    height={Math.max(0, barHeight)}
                    rx={4}
                    fill={s.color}
                  />
                )
              })}
              {/* Hit target selebar grup, lebih besar dari bar-nya sendiri. */}
              <rect
                x={groupX}
                y={PAD_TOP}
                width={groupWidth}
                height={plotHeight}
                fill="transparent"
                onMouseEnter={() => setHoverIndex(i)}
                onMouseLeave={() => setHoverIndex((cur) => (cur === i ? null : cur))}
                style={{ cursor: 'pointer' }}
              />
              <text x={groupCenter} y={HEIGHT - 8} fontSize="9" textAnchor="middle" fill="currentColor" fillOpacity="0.6">
                {d.month.slice(2, 7)}
              </text>
            </g>
          )
        })}
      </svg>

      {hovered && (
        <div
          className="position-absolute bg-body border rounded shadow-sm px-2 py-1 small"
          style={{
            left: `${((PAD_LEFT + (hoverIndex + 0.5) * groupWidth) / WIDTH) * 100}%`,
            top: 0,
            transform: `translate(${hoverIndex < data.length * 0.15 ? '0' : hoverIndex > data.length * 0.85 ? '-100%' : '-50%'}, -100%)`,
            whiteSpace: 'nowrap',
            pointerEvents: 'none',
            zIndex: 1,
          }}
        >
          <div className="fw-semibold mb-1">{hovered.month}</div>
          {SERIES.map((s) => (
            <div key={s.key} className="d-flex align-items-center gap-1">
              <span style={{ width: 8, height: 2, background: s.color, display: 'inline-block' }} />
              <span className="text-secondary">{s.label}:</span> {formatValue(hovered[s.key])}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default RevenueExpenseBarChart
