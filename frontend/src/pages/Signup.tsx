import type { ChangeEvent, FormEvent } from 'react'
import { useDeferredValue, useEffect, useState } from 'react'
import { FaEye, FaEyeSlash } from 'react-icons/fa6'
import { Link, useNavigate } from 'react-router-dom'
import AuthField from '../components/AuthField'
import AuthLayout from '../components/AuthLayout'
import SSOButtons from '../components/SSOButtons'
import { ApiError, fallbackSSOProviders, listSSOProviders, saveSession, signUp, type SSOProvider } from '../lib/api'
import { evaluatePasswordStrength } from '../lib/password'

export default function SignUp() {
  const navigate = useNavigate()
  const [form, setForm] = useState({ fullName: '', email: '', password: '' })
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [providers, setProviders] = useState<SSOProvider[]>(fallbackSSOProviders)
  const deferredPassword = useDeferredValue(form.password)
  const strength = evaluatePasswordStrength(deferredPassword)

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

  const update = (key: 'fullName' | 'email' | 'password') => (event: ChangeEvent<HTMLInputElement>) => {
    setForm((current) => ({ ...current, [key]: event.target.value }))
  }

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setNotice('')
    setLoading(true)

    try {
      const session = await signUp(form)
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
      title='Create your account'
      subtitle='Provision your workspace and step into the dashboard in seconds.'
      footer={<>Already have an account? <Link to='/sign-in'>Sign in</Link></>}
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
          label='Full Name'
          value={form.fullName}
          autoComplete='name'
          onChange={update('fullName')}
        />

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
          autoComplete='new-password'
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

        <div className='password-strength'>
          <div className='password-strength__meter' aria-hidden='true'>
            {Array.from({ length: 4 }, (_, index) => (
              <span
                key={index}
                className={index < strength.score ? 'password-strength__segment password-strength__segment--active' : 'password-strength__segment'}
              />
            ))}
          </div>
          <p className='password-strength__label'>{strength.label}</p>
          <p className='password-strength__hint'>{strength.hint}</p>
        </div>

        <button className='auth-submit' type='submit' disabled={loading}>
          {loading ? 'Creating account...' : 'Create Account'}
        </button>
      </form>
    </AuthLayout>
  )
}
