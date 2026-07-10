function StatTile({ icon, label, value, hint, color = 'primary' }) {
  return (
    <div className="col-12 col-sm-6 col-xl-3">
      <div className="stat-tile">
        <div className={`stat-tile-icon stat-tile-icon-${color}`}>
          <i className={`bi ${icon}`} />
        </div>
        <div>
          <div className="stat-tile-label">{label}</div>
          <div className="stat-tile-value">{value}</div>
          {hint && <div className="stat-tile-hint">{hint}</div>}
        </div>
      </div>
    </div>
  )
}

export default StatTile
