import { startTransition, useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import AuthLayout from '../components/AuthLayout'
import { ApiError, resolveSessionFromToken, saveSession } from '../lib/api'

export default function AuthCallback() {
  const location = useLocation()
  const navigate = useNavigate()
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false

    const finishSignIn = async () => {
      const params = parseAuthCallbackParams(location)
      const errorMessage = params.get('error')?.trim()
      if (errorMessage) {
        if (!cancelled) {
          setError(errorMessage)
        }
        return
      }

      const token = params.get('token')?.trim() ?? ''
      const returnUrl = sanitizeReturnUrl(params.get('return_url'))
      if (!token) {
        if (!cancelled) {
          setError('The sign-in callback did not include a session token.')
        }
        return
      }

      try {
        const session = await resolveSessionFromToken(token)
        if (cancelled) {
          return
        }

        saveSession(session)
        startTransition(() => {
          navigate(returnUrl, { replace: true })
        })
      } catch (reason) {
        if (cancelled) {
          return
        }
        setError(reason instanceof ApiError ? reason.message : 'Could not finish signing you in right now.')
      }
    }

    void finishSignIn()

    return () => {
      cancelled = true
    }
  }, [location, navigate])

  return (
    <AuthLayout
      title='Finishing sign-in'
      subtitle='BabelSuite is validating your identity and starting your local session.'
      footer={error ? <>Want to try again? <Link to='/sign-in'>Back to sign in</Link></> : undefined}
    >
      {error ? (
        <div className='auth-message auth-message--error'>{error}</div>
      ) : (
        <div className='auth-message auth-message--info'>Completing single sign-on...</div>
      )}
    </AuthLayout>
  )
}

function parseAuthCallbackParams(location: { hash: string; search: string }) {
  const fragment = location.hash.startsWith('#') ? location.hash.slice(1) : location.hash
  if (fragment) {
    return new URLSearchParams(fragment)
  }
  return new URLSearchParams(location.search)
}

function sanitizeReturnUrl(candidate: string | null) {
  if (!candidate) {
    return '/'
  }

  const value = candidate.trim()
  if (value.startsWith('/') && !value.startsWith('//')) {
    return value
  }
  return '/'
}

