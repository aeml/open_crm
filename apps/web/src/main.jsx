import React from 'react'
import ReactDOM from 'react-dom/client'
import { AppRouter } from './app/router'
import { AppErrorBoundary } from './components/ui/error_boundary'
import './styles/tokens.css'
import './styles/base.css'
import './styles/layout.css'
import './styles/components.css'

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <AppRouter />
    </AppErrorBoundary>
  </React.StrictMode>
)
