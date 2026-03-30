import type { ChangeEvent, FormEvent } from 'react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import AuthField from '../components/AuthField'
import AuthLayout from '../components/AuthLayout'

export default function ForgotPassword() {
  const [email, setEmail] = useState('')
  const [submitted, setSubmitted] = useState(false)

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSubmitted(true)
  }

  return (
    <AuthLayout
      title='Reset your password'
      subtitle='This route is ready for the next backend step. For now it gives users a safe recovery path instead of a dead end.'
      footer={<>Back to <Link to='/sign-in'>Sign in</Link></>}
    >
      {submitted && (
        <div className='auth-message auth-message--info'>
          Password reset delivery is the next backend endpoint to wire up. Use this page as the UX placeholder for now.
        </div>
      )}

      <form className='auth-form' onSubmit={submit}>
        <AuthField
          label='Email Address'
          type='email'
          value={email}
          autoComplete='email'
          onChange={(event: ChangeEvent<HTMLInputElement>) => setEmail(event.target.value)}
        />

        <button className='auth-submit' type='submit'>
          Send Reset Link
        </button>
      </form>
    </AuthLayout>
  )
}
