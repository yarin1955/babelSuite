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

        if (provider.enabled && provider.startUrl) {
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

function resolveReturnURL() {
  const currentUrl = new URL(window.location.href)
  const explicit = currentUrl.searchParams.get('returnTo')?.trim()
  if (explicit && explicit.startsWith('/') && !explicit.startsWith('//')) {
    return explicit
  }
  return '/'
}
