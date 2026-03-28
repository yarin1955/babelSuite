import type { ChangeEvent, FormEvent } from 'react'
import { useEffect, useState } from 'react'
import { FaEye, FaEyeSlash } from 'react-icons/fa6'
import { Link, useNavigate } from 'react-router-dom'
import AuthField from '../components/AuthField'
import AuthLayout from '../components/AuthLayout'
import SSOButtons from '../components/SSOButtons'
import { ApiError, fallbackSSOProviders, listSSOProviders, saveSession, signIn, type SSOProvider } from '../lib/api'

export default function SignIn() {
  const navigate = useNavigate()
  const [form, setForm] = useState({ email: '', password: '' })
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [providers, setProviders] = useState<SSOProvider[]>(fallbackSSOProviders)

  useEffect(() => {
    let cancelled = false

    listSSOProviders()
      .then((items) => {
        if (!cancelled && items.length > 0) {
          setProviders(items)
        }
      })
      .catch(() => {})

    return () => {
      cancelled = true
    }
  }, [])

  const update = (key: 'email' | 'password') => (event: ChangeEvent<HTMLInputElement>) => {
    setForm((current) => ({ ...current, [key]: event.target.value }))
  }

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setNotice('')
    setLoading(true)

    try {
      const session = await signIn(form)
      saveSession(session)
      navigate('/')
    } catch (reason) {
      setError(reason instanceof ApiError ? reason.message : 'Cannot reach the authentication service right now.')
    } finally {
      setLoading(false)
    }
  }

  const showProviderHint = (provider: SSOProvider) => {
    setError('')
    setNotice(provider.hint ?? `${provider.name} SSO is not configured yet.`)
  }

  return (
    <AuthLayout
      title='Sign in'
      subtitle='Access your BabelSuite environment and execution history.'
      footer={<>Don't have an account? <Link to='/sign-up'>Sign up</Link></>}
    >
      <SSOButtons providers={providers} onUnavailable={showProviderHint} />

      <div className='auth-divider'><span>OR</span></div>

      {(error || notice) && (
        <div className={`auth-message ${error ? 'auth-message--error' : 'auth-message--info'}`}>
          {error || notice}
        </div>
      )}

      <form className='auth-form' onSubmit={submit}>
        <AuthField
          label='Email Address'
          type='email'
          value={form.email}
          autoComplete='email'
          onChange={update('email')}
        />

        <AuthField
          label='Password'
          type={showPassword ? 'text' : 'password'}
          value={form.password}
          autoComplete='current-password'
          onChange={update('password')}
          trailing={(
            <button
              type='button'
              className='auth-field__toggle'
              onClick={() => setShowPassword((current) => !current)}
              aria-label={showPassword ? 'Hide password' : 'Show password'}
            >
              {showPassword ? <FaEyeSlash /> : <FaEye />}
            </button>
          )}
        />

        <div className='auth-inline-row'>
          <Link className='auth-link auth-link--subtle' to='/forgot-password'>
            Forgot your password?
          </Link>
        </div>

        <button className='auth-submit' type='submit' disabled={loading}>
          {loading ? 'Signing in...' : 'Sign In'}
        </button>
      </form>
    </AuthLayout>
  )
}
