import { FaShieldHalved } from 'react-icons/fa6'
import type { ProfileRecord } from '../../lib/api'
import './ProfileCard.css'

export function ProfileCard({
  profile,
  selected,
  onOpen,
}: {
  profile: ProfileRecord
  selected: boolean
  onOpen: (profile: ProfileRecord) => void
}) {
  return (
    <button
      type='button'
      className={[
        'profile-card',
        profile.default ? 'profile-card--default' : '',
        !profile.launchable ? 'profile-card--base' : '',
        selected ? 'profile-card--selected' : '',
      ].filter(Boolean).join(' ')}
      onClick={() => onOpen(profile)}
    >
      <div className='profile-card__body'>
        <div className='profile-card__top'>
          <span className='profile-card__name'>{profile.name}</span>
          <div className='profile-card__badges'>
            {profile.default && <span className='profile-badge profile-badge--default'>Default</span>}
            {!profile.launchable && <span className='profile-badge profile-badge--base'>Base</span>}
          </div>
        </div>
        <p className='profile-card__filename'>{profile.fileName}</p>
        {profile.description && (
          <p className='profile-card__desc'>{profile.description}</p>
        )}
      </div>
      <div className='profile-card__footer'>
        <span className='profile-card__scope'>{profile.scope}</span>
        {profile.secretRefs.length > 0 && (
          <span className='profile-card__secrets'>
            <FaShieldHalved />
            {profile.secretRefs.length}
          </span>
        )}
        <span className='profile-card__date'>
          {profile.updatedAt ? new Date(profile.updatedAt).toLocaleDateString() : ''}
        </span>
      </div>
    </button>
  )
}
