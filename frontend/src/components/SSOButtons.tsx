import type { ReactElement } from 'react'
import { FaArrowRightToBracket, FaGithub, FaGitlab } from 'react-icons/fa6'
import type { SSOProvider } from '../lib/api'

interface SSOButtonsProps {
  providers: SSOProvider[]
  onUnavailable: (provider: SSOProvider) => void
}

const icons: Record<string, ReactElement> = {
  github: <FaGithub />,
  gitlab: <FaGitlab />,
  oidc: <FaArrowRightToBracket />,
}

export default function SSOButtons({ providers, onUnavailable }: SSOButtonsProps) {
  return (
    <div className='auth-sso'>
      {providers.map((provider) => {
        const content = (
          <>
            <span className='auth-sso__icon'>{icons[provider.providerId] ?? <FaGithub />}</span>
            <span>{provider.buttonLabel}</span>
          </>
        )

        if (provider.enabled && provider.startUrl && isSafeUrl(provider.startUrl)) {
          const loginUrl = new URL(provider.startUrl)
          loginUrl.searchParams.set('return_url', resolveReturnURL())

          return (
            <a
              key={provider.providerId}
              className={`auth-sso__button auth-sso__button--${provider.providerId}`}
              href={loginUrl.toString()}
            >
              {content}
            </a>
          )
        }

        return (
          <button
            key={provider.providerId}
            type='button'
            className={`auth-sso__button auth-sso__button--${provider.providerId} auth-sso__button--disabled`}
            onClick={() => onUnavailable(provider)}
          >
            {content}
          </button>
        )
      })}
    </div>
  )
}

function isSafeUrl(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function resolveReturnURL() {
  const currentUrl = new URL(window.location.href)
  const explicit = currentUrl.searchParams.get('returnTo')?.trim()
  if (explicit && explicit.startsWith('/') && !explicit.startsWith('//')) {
    return explicit
  }
  const path = window.location.pathname + window.location.search
  return path === '/sign-in' || path === '/sign-up' ? '/' : path
}
