import { useMemo, useState } from 'react'

const WIDTH = 560
const HEIGHT = 200
const PAD_LEFT = 8
const PAD_RIGHT = 8
const PAD_TOP = 16
const PAD_BOTTOM = 28

// Line chart minimal: history (solid) + forecast (dashed), satu hue yang
// sama supaya jelas ini satu seri yang sama (aktual vs proyeksi), bukan dua
// entitas berbeda -- identitas dibedakan lewat gaya garis (legend), bukan
// warna. Crosshair + tooltip di-hover sesuai pola interaksi wajib untuk
// chart garis (lihat dataviz skill).
function ForecastLineChart({ history, forecast, formatValue, color = 'var(--bs-primary)' }) {
  const [hoverIndex, setHoverIndex] = useState(null)

  const points = useMemo(() => {
    const historyPoints = history.map((p) => ({ ...p, isForecast: false }))
    const forecastPoints = forecast.map((p) => ({ ...p, isForecast: true }))
    return [...historyPoints, ...forecastPoints]
  }, [history, forecast])

  if (points.length === 0) {
    return <div className="text-secondary small">Belum ada data.</div>
  }

  const values = points.map((p) => p.value)
  const minValue = Math.min(0, ...values)
  const maxValue = Math.max(1, ...values)
  const plotWidth = WIDTH - PAD_LEFT - PAD_RIGHT
  const plotHeight = HEIGHT - PAD_TOP - PAD_BOTTOM

  const xForIndex = (i) => PAD_LEFT + (points.length === 1 ? plotWidth / 2 : (i / (points.length - 1)) * plotWidth)
  const yForValue = (v) => PAD_TOP + plotHeight - ((v - minValue) / (maxValue - minValue || 1)) * plotHeight

  const historyCount = history.length
  const linePath = (subset, startIdx) =>
    subset.map((p, i) => `${i === 0 ? 'M' : 'L'} ${xForIndex(startIdx + i)} ${yForValue(p.value)}`).join(' ')

  const historyPath = linePath(history, 0)
  // Sambungkan garis forecast dari titik terakhir history supaya menyatu.
  const forecastPath =
    forecast.length > 0
      ? linePath([history[historyCount - 1], ...forecast], historyCount - 1)
      : ''

  const hovered = hoverIndex != null ? points[hoverIndex] : null

  return (
    <div className="position-relative">
      <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} style={{ width: '100%', height: 'auto', overflow: 'visible' }}>
        <line x1={PAD_LEFT} y1={PAD_TOP + plotHeight} x2={WIDTH - PAD_RIGHT} y2={PAD_TOP + plotHeight} stroke="currentColor" strokeOpacity="0.15" strokeWidth="1" />

        {historyPath && <path d={historyPath} fill="none" stroke={color} strokeWidth="2" />}
        {forecastPath && <path d={forecastPath} fill="none" stroke={color} strokeWidth="2" strokeDasharray="5,4" strokeOpacity="0.7" />}

        {points.map((p, i) => (
          <circle
            key={i}
            cx={xForIndex(i)}
            cy={yForValue(p.value)}
            r={i === hoverIndex ? 5 : 3.5}
            fill={p.isForecast ? 'var(--bs-body-bg)' : color}
            stroke={color}
            strokeWidth="2"
          />
        ))}

        {hovered && (
          <line x1={xForIndex(hoverIndex)} y1={PAD_TOP} x2={xForIndex(hoverIndex)} y2={PAD_TOP + plotHeight} stroke="currentColor" strokeOpacity="0.25" strokeWidth="1" />
        )}

        {points.map((p, i) => (
          <rect
            key={`hit-${i}`}
            x={xForIndex(i) - plotWidth / points.length / 2}
            y={PAD_TOP}
            width={plotWidth / points.length}
            height={plotHeight}
            fill="transparent"
            onMouseEnter={() => setHoverIndex(i)}
            onMouseLeave={() => setHoverIndex((cur) => (cur === i ? null : cur))}
            style={{ cursor: 'pointer' }}
          />
        ))}

        {points.map((p, i) => (
          <text key={`label-${i}`} x={xForIndex(i)} y={HEIGHT - 8} fontSize="9" textAnchor="middle" fill="currentColor" fillOpacity="0.6">
            {p.period.slice(2)}
          </text>
        ))}
      </svg>

      {hovered && (
        <div
          className="position-absolute bg-body border rounded shadow-sm px-2 py-1 small"
          style={{
            left: `${(xForIndex(hoverIndex) / WIDTH) * 100}%`,
            top: 0,
            // Dekat tepi kiri/kanan, anchor ke sisi itu supaya tooltip tidak
            // meluber keluar kartu; di tengah tetap center di atas titik.
            transform: `translate(${hoverIndex < points.length * 0.15 ? '0' : hoverIndex > points.length * 0.85 ? '-100%' : '-50%'}, -100%)`,
            whiteSpace: 'nowrap',
            pointerEvents: 'none',
            zIndex: 1,
          }}
        >
          <div className="fw-semibold">{hovered.period}</div>
          <div>{formatValue(hovered.value)} {hovered.isForecast && <span className="text-secondary">(proyeksi)</span>}</div>
        </div>
      )}

      <div className="d-flex gap-3 small text-secondary mt-1">
        <span className="d-flex align-items-center gap-1">
          <svg width="16" height="2"><line x1="0" y1="1" x2="16" y2="1" stroke={color} strokeWidth="2" /></svg>
          Aktual
        </span>
        <span className="d-flex align-items-center gap-1">
          <svg width="16" height="2"><line x1="0" y1="1" x2="16" y2="1" stroke={color} strokeWidth="2" strokeDasharray="4,3" strokeOpacity="0.7" /></svg>
          Proyeksi
        </span>
      </div>
    </div>
  )
}

export default ForecastLineChart
