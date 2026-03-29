import { useEffect, useState } from 'react'
import { FaAngleRight, FaBoxArchive, FaGear, FaKey, FaServer } from 'react-icons/fa6'
import { useNavigate } from 'react-router-dom'
import { getPlatformSettings, type PlatformSettings } from '../lib/api'
import AppShell from '../components/AppShell'
import './PlatformSettings.css'

export default function Settings() {
  const navigate = useNavigate()
  const [platform, setPlatform] = useState<PlatformSettings | null>(null)

  useEffect(() => {
    void getPlatformSettings().then(setPlatform).catch(() => null)
  }, [])

  const sections = [
    {
      path: '/settings/general',
      icon: FaGear,
      title: 'General',
      description: platform
        ? `${platform.mode} mode — last saved ${platform.updatedAt ? new Date(platform.updatedAt).toLocaleDateString() : 'never'}`
        : 'Platform deployment mode and instance description.',
    },
    {
      path: '/settings/agents',
      icon: FaServer,
      title: 'Execution Agents',
      description: platform
        ? `${platform.agents.length} configured agent${platform.agents.length !== 1 ? 's' : ''} — local, remote Docker, and Kubernetes targets`
        : 'Configure environments where suites execute.',
    },
    {
      path: '/settings/registries',
      icon: FaBoxArchive,
      title: 'OCI Registries',
      description: platform
        ? `${platform.registries.length} upstream source${platform.registries.length !== 1 ? 's' : ''} with manual sync and provider auth`
        : 'Control where BabelSuite discovers suites and modules.',
    },
    {
      path: '/settings/secrets',
      icon: FaKey,
      title: 'Global Secrets',
      description: platform
        ? `${platform.secrets.globalOverrides.length} global override${platform.secrets.globalOverrides.length !== 1 ? 's' : ''} — ${platform.secrets.provider === 'none' ? 'no external secrets manager' : platform.secrets.provider}`
        : 'Configure Vault or AWS Secrets Manager and global overrides.',
    },
  ]

  return (
    <AppShell
      section='Settings'
      title='Platform Administration'
      description='Configure BabelSuite runtime targets, catalog sources, and secret management.'
      actions={platform ? (
        <span className='platform-badge'>{platform.mode} mode</span>
      ) : undefined}
    >
      <div className='platform-page'>
        <div className='settings-index'>
          {sections.map((section) => {
            const Icon = section.icon
            return (
              <button
                key={section.path}
                className='settings-index__card'
                onClick={() => navigate(section.path)}
              >
                <div className='settings-index__icon'>
                  <Icon />
                </div>
                <div className='settings-index__body'>
                  <strong>{section.title}</strong>
                  <p>{section.description}</p>
                </div>
                <FaAngleRight className='settings-index__arrow' />
              </button>
            )
          })}
        </div>
      </div>
    </AppShell>
  )
}
