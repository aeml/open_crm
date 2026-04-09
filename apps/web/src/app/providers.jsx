import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { API_BASE_URL } from '../lib/config'

const AuthContext = createContext({
  status: 'unauthenticated',
  session: null,
  businessProfile: null,
  error: '',
  login: async () => {
    throw new Error('Unable to sign in.')
  },
  bootstrap: async () => {
    throw new Error('Unable to create workspace.')
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

    setSession(payload.data)
    setBusinessProfile(null)
    setStatus('authenticated')
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
      bootstrap,
      refreshSession,
      setBusinessProfile
    }),
    [bootstrap, businessProfile, error, login, refreshSession, session, status]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  return useContext(AuthContext)
}
