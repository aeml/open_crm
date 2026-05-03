import { Component } from 'react'

export class AppErrorBoundary extends Component {
  constructor(props) {
    super(props)
    this.state = { hasError: false, message: '' }
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, message: error?.message || 'An unexpected error occurred.' }
  }

  componentDidCatch(error, info) {
    console.error('AppErrorBoundary caught error:', error, info?.componentStack)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="auth-layout">
          <div className="auth-card card">
            <div className="card-stack">
              <div>
                <p className="eyebrow">Error</p>
                <h2>Something went wrong</h2>
                <p className="page-description">{this.state.message}</p>
              </div>
              <div>
                <button
                  className="button button-primary"
                  type="button"
                  onClick={() => window.location.reload()}
                >
                  Reload page
                </button>
              </div>
            </div>
          </div>
        </div>
      )
    }

    return this.props.children
  }
}
