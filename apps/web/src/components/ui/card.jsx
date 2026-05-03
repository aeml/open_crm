export function Card({ children, className = '' }) {
  const classes = ['card', className].filter(Boolean).join(' ')
  return <div className={classes}>{children}</div>
}
