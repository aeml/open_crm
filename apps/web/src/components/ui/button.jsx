export function Button({ children, type = 'button', className = '', ...props }) {
  return (
    <button className={['button', 'button-primary', className].filter(Boolean).join(' ')} type={type} {...props}>
      {children}
    </button>
  )
}
