import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

const API = 'http://localhost:8090'

export default function Signup() {
  const nav = useNavigate()
  const [form, setForm]     = useState({ name: '', email: '', username: '', password: '' })
  const [show, setShow]     = useState(false)
  const [err, setErr]       = useState('')
  const [loading, setLoading] = useState(false)

  const set = (k: string) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(f => ({ ...f, [k]: e.target.value }))

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setErr('')
    setLoading(true)
    try {
      const res = await fetch(`${API}/auth/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      const data = await res.json()
      if (!res.ok) { setErr(data.error || 'Registration failed'); return }
      localStorage.setItem('token', data.token)
      localStorage.setItem('user',  JSON.stringify(data.user))
      nav('/runs')
    } catch {
      setErr('Cannot reach server')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className='auth-page'>
      <div className='auth-card'>
        <h1 className='auth-card__title'>Sign up</h1>

        {err && <div className='auth-error'>{err}</div>}

        <form onSubmit={submit} noValidate>
          <Field label='Name'     value={form.name}     onChange={set('name')} />
          <Field label='Username' value={form.username} onChange={set('username')} />
          <Field label='Email'    value={form.email}    onChange={set('email')} type='email' />
          <Field
            label='Password' value={form.password} onChange={set('password')}
            type={show ? 'text' : 'password'}
            icon={
              <span className='field__icon' onClick={() => setShow(s => !s)}>
                {show ? <EyeOff /> : <Eye />}
              </span>
            }
          />

          <button className='btn btn--primary' type='submit' disabled={loading} style={{ marginTop: 8 }}>
            {loading ? 'Creating account…' : 'Sign Up'}
          </button>
        </form>

        <div className='auth-switch'>
          Already have an account? <Link to='/login'>Log In</Link>
        </div>

        <div className='divider'>or</div>

        <button className='btn btn--google' type='button'>
          <GoogleIcon /> Sign up with Google
        </button>
      </div>
    </div>
  )
}

// ── tiny shared components ────────────────────────────────────────────────────

function Field({ label, value, onChange, type = 'text', icon }: {
  label: string; value: string; type?: string
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  icon?: React.ReactNode
}) {
  return (
    <div className='field'>
      <input
        className='field__input'
        type={type}
        placeholder=' '
        value={value}
        onChange={onChange}
        autoComplete={type === 'password' ? 'new-password' : undefined}
      />
      <label className='field__label'>{label}</label>
      {icon}
    </div>
  )
}

function Eye() {
  return (
    <svg width='18' height='18' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
      <path d='M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z'/>
      <circle cx='12' cy='12' r='3'/>
    </svg>
  )
}

function EyeOff() {
  return (
    <svg width='18' height='18' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2'>
      <path d='M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24'/>
      <line x1='1' y1='1' x2='23' y2='23'/>
    </svg>
  )
}

function GoogleIcon() {
  return (
    <svg width='18' height='18' viewBox='0 0 24 24'>
      <path fill='#4285F4' d='M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z'/>
      <path fill='#34A853' d='M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z'/>
      <path fill='#FBBC05' d='M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z'/>
      <path fill='#EA4335' d='M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z'/>
    </svg>
  )
}
