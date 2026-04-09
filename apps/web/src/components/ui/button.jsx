export function Button({ children, type = 'button' }) {
  return (
    <button className="button button-primary" type={type}>
      {children}
    </button>
  )
}
