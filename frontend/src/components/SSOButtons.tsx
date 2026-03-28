import type { ReactElement } from 'react'
import { FaGithub, FaGitlab } from 'react-icons/fa6'
import type { SSOProvider } from '../lib/api'

interface SSOButtonsProps {
  providers: SSOProvider[]
  onUnavailable: (provider: SSOProvider) => void
}

const icons: Record<string, ReactElement> = {
  github: <FaGithub />,
  gitlab: <FaGitlab />,
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
          return (
            <a
              key={provider.providerId}
              className={`auth-sso__button auth-sso__button--${provider.providerId}`}
              href={provider.startUrl}
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
