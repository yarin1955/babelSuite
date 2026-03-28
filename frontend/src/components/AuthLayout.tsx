import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

interface AuthLayoutProps {
  title: string
  subtitle: string
  children: ReactNode
  footer: ReactNode
}

export default function AuthLayout({ title, subtitle, children, footer }: AuthLayoutProps) {
  return (
    <div className='auth-page'>
      <div className='auth-page__glow auth-page__glow--blue' />
      <div className='auth-page__glow auth-page__glow--amber' />
      <div className='auth-page__grid' />

      <main className='auth-card'>
        <Link className='auth-card__brand' to='/sign-in' aria-label='BabelSuite home'>
          <span className='auth-brand-mark'>BS</span>
          <span className='auth-brand-name'>BabelSuite</span>
        </Link>

        <div className='auth-card__intro'>
          <h1 className='auth-card__title'>{title}</h1>
          <p className='auth-card__subtitle'>{subtitle}</p>
        </div>

        {children}

        <div className='auth-card__footer'>{footer}</div>
      </main>
    </div>
  )
}
