function Logo({ size = 30, withText = true }) {
  return (
    <span className="edp-logo">
      <span className="edp-logo-mark" style={{ width: size, height: size, fontSize: size * 0.55 }}>
        <i className="bi bi-hexagon-fill" />
      </span>
      {withText && <span className="edp-logo-text">Enterprise Digital Platform</span>}
    </span>
  )
}

export default Logo
