import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { API_BASE_URL } from '../lib/config'

const AuthContext = createContext({
  status: 'unauthenticated',
  session: null,
  businessProfile: null,
  error: '',
  login: async () => {
    throw new Error('Unable to sign in.')
  },
  logout: async () => {
    throw new Error('Unable to sign out.')
  },
  bootstrap: async () => {
    throw new Error('Unable to create workspace.')
  },
  verifyEmail: async () => {
    throw new Error('Unable to verify workspace email.')
  },
  resendVerification: async () => {
    throw new Error('Unable to send verification email.')
  },
  refreshSession: async () => null,
  setBusinessProfile: () => {}
})

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

export function AppProviders({ children }) {
  const [status, setStatus] = useState('checking')
  const [session, setSession] = useState(null)
  const [businessProfile, setBusinessProfile] = useState(null)
  const [error, setError] = useState('')
  const statusRef = useRef(status)
  useEffect(() => {
    statusRef.current = status
  }, [status])

  useEffect(() => {
    function handleUnauthorized() {
      if (statusRef.current === 'authenticated') {
        setStatus('unauthenticated')
        setSession(null)
        setBusinessProfile(null)
      }
    }

    window.addEventListener('auth:unauthorized', handleUnauthorized)
    return () => window.removeEventListener('auth:unauthorized', handleUnauthorized)
  }, [])

  const refreshSession = useCallback(async () => {
    if (typeof fetch !== 'function') {
      setStatus('unauthenticated')
      setSession(null)
      return null
    }

    setStatus('checking')
    setError('')

    try {
      const response = await fetch(`${API_BASE_URL}/auth/me`, {
        credentials: 'include'
      })
      const payload = await response.json()

      if (!response.ok) {
        if (response.status === 401) {
          setStatus('unauthenticated')
          setSession(null)
          setBusinessProfile(null)
          return null
        }

        throw new Error(getErrorMessage(payload, 'Unable to load your session.'))
      }

      setSession(payload.data)
      setBusinessProfile(null)
      setStatus('authenticated')
      return payload.data
    } catch (refreshError) {
      setStatus('unauthenticated')
      setSession(null)
      setBusinessProfile(null)
      setError(refreshError.message || 'Unable to load your session.')
      return null
    }
  }, [])

  useEffect(() => {
    refreshSession()
  }, [refreshSession])

  const login = useCallback(async ({ email, password }) => {
    const response = await fetch(`${API_BASE_URL}/auth/login`, {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ email, password })
    })

    const payload = await response.json()

    if (!response.ok) {
      const message = getErrorMessage(payload, 'Unable to sign in.')
      setStatus('unauthenticated')
      setSession(null)
      setBusinessProfile(null)
      setError(message)
      throw new Error(message)
    }

    setSession(payload.data)
    setBusinessProfile(null)
    setStatus('authenticated')
    setError('')
    return payload.data
  }, [])

  const verifyEmail = useCallback(async (token) => {
    const response = await fetch(`${API_BASE_URL}/auth/verify-email`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token })
    })
    const payload = await response.json()
    if (!response.ok) {
      const message = getErrorMessage(payload, 'Unable to verify workspace email.')
      setStatus('unauthenticated')
      setSession(null)
      setBusinessProfile(null)
      setError(message)
      throw new Error(message)
    }
    setSession(payload.data)
    setBusinessProfile(null)
    setStatus('authenticated')
    setError('')
    window.sessionStorage.removeItem('open-crm-bootstrap-key')
    return payload.data
  }, [])

  const resendVerification = useCallback(async (email) => {
    const response = await fetch(`${API_BASE_URL}/auth/resend-verification`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email })
    })
    const payload = await response.json()
    if (!response.ok) {
      const message = getErrorMessage(payload, 'Unable to send verification email.')
      setError(message)
      throw new Error(message)
    }
    setError('')
    return payload.data
  }, [])

  const logout = useCallback(async () => {
    const response = await fetch(`${API_BASE_URL}/auth/logout`, {
      method: 'POST',
      credentials: 'include'
    })

    const payload = await response.json().catch(() => ({}))

    if (!response.ok && response.status !== 401) {
      const message = getErrorMessage(payload, 'Unable to sign out.')
      setError(message)
      throw new Error(message)
    }

    setStatus('unauthenticated')
    setSession(null)
    setBusinessProfile(null)
    setError('')
  }, [])

  const bootstrap = useCallback(async (input) => {
    const response = await fetch(`${API_BASE_URL}/auth/bootstrap`, {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(input)
    })

    const payload = await response.json()

    if (!response.ok) {
      const message = getErrorMessage(payload, 'Unable to create workspace.')
      setStatus('unauthenticated')
      setSession(null)
      setBusinessProfile(null)
      setError(message)
      throw new Error(message)
    }

    setSession(null)
    setBusinessProfile(null)
    setStatus('unauthenticated')
    setError('')
    return payload.data
  }, [])

  const value = useMemo(
    () => ({
      status,
      session,
      businessProfile,
      error,
      login,
      logout,
      bootstrap,
      verifyEmail,
      resendVerification,
      refreshSession,
      setBusinessProfile
    }),
    [bootstrap, businessProfile, error, login, logout, refreshSession, resendVerification, session, status, verifyEmail]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  return useContext(AuthContext)
}
