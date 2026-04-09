import {
  FaCircleCheck,
  FaCopy,
  FaCubes,
  FaDiagramProject,
  FaDownload,
  FaRegStar,
  FaServer,
  FaShieldHalved,
  FaStar,
} from 'react-icons/fa6'
import type { CatalogPackage } from '../lib/api'
import { logoGradient } from './logoGradient'

export interface PackageCardProps {
  item: CatalogPackage
  starred: boolean
  copiedId: string
  favoriteBusy: boolean
  onInspect: () => void
  onCopyRun: () => void
  onCopyFork: () => void
  onToggleFavorite: () => void
}

export function PackageCard({ item, starred, copiedId, favoriteBusy, onInspect, onCopyRun, onCopyFork, onToggleFavorite }: PackageCardProps) {
  const runCopied = copiedId === item.id + '-run'
  const forkCopied = copiedId === item.id + '-fork'
  const visibleModules = item.modules.slice(0, 3)
  const hiddenModuleCount = Math.max(item.modules.length - visibleModules.length, 0)
  const publishedVersions = countPublishedVersions(item.tags)
  const versionSummary = publishedVersions > 1 ? `${publishedVersions} versions available` : '1 version available'

  return (
    <article className='catalog-card'>
      {/* Header row: logo + headline + star */}
      <div className='catalog-card__header'>
        <div className='catalog-card__logo' aria-hidden='true' style={{ background: logoGradient(item.id) }}>
          {item.title.slice(0, 2).toUpperCase()}
        </div>

        <div className='catalog-card__headline'>
          <div className='catalog-card__title-row'>
            <h2 className='catalog-card__title'>{item.title}</h2>
            {item.status === 'Official' && (
              <span className='catalog-badge catalog-badge--official'>
                <FaShieldHalved /> Official
              </span>
            )}
            {item.status === 'Verified' && (
              <span className='catalog-badge catalog-badge--verified'>
                <FaCircleCheck /> Verified
              </span>
            )}
          </div>
          <p className='catalog-card__origin'>
            <span>{item.owner}</span>
            <span className='catalog-card__sep' aria-hidden='true'> · </span>
            <span>{item.repository}</span>
            <span className='catalog-card__sep' aria-hidden='true'> · </span>
            <strong className='catalog-card__version'>{item.version}</strong>
          </p>
        </div>

        <button
          type='button'
          className={`catalog-card__star${starred ? ' catalog-card__star--active' : ''}`}
          aria-pressed={starred}
          aria-label={starred ? `Unstar ${item.title}` : `Star ${item.title}`}
          title={starred ? 'Unstar' : 'Star'}
          disabled={favoriteBusy}
          onClick={onToggleFavorite}
        >
          {starred ? <FaStar /> : <FaRegStar />}
        </button>
      </div>

      {/* Description */}
      <p className='catalog-card__description'>{item.description}</p>

      {/* Module + tag pills */}
      {visibleModules.length > 0 && (
        <div className='catalog-card__pills'>
          {visibleModules.map((m) => (
            <span key={m} className='catalog-pill catalog-pill--module'>
              <FaCubes /> {m.replace('@babelsuite/', '')}
            </span>
          ))}
          {hiddenModuleCount > 0 && <span className='catalog-pill catalog-pill--more'>+{hiddenModuleCount} more</span>}
        </div>
      )}

      {/* Footer: source + actions */}
      <div className='catalog-card__footer'>
        <div className='catalog-card__meta'>
          <span className='catalog-card__provider'>
            <FaServer /> {item.provider}
          </span>
          <span className='catalog-card__versions'>{versionSummary}</span>
        </div>

        <div className='catalog-card__actions'>
          {item.kind === 'suite' && (
            <button type='button' className='catalog-btn catalog-btn--primary' onClick={onInspect}>
              <FaDiagramProject />
              <span>Inspect</span>
            </button>
          )}
          <button
            type='button'
            className={`catalog-btn${runCopied ? ' catalog-btn--copied' : ' catalog-btn--secondary'}`}
            onClick={onCopyRun}
          >
            <FaCopy />
            <span>{runCopied ? 'Copied!' : 'Run command'}</span>
          </button>
          <button
            type='button'
            className={`catalog-btn${forkCopied ? ' catalog-btn--copied' : ' catalog-btn--ghost'}`}
            onClick={onCopyFork}
          >
            <FaDownload />
            <span>{forkCopied ? 'Copied!' : 'Fork'}</span>
          </button>
        </div>
      </div>
    </article>
  )
}

function countPublishedVersions(tags: string[]): number {
  const stableTags = new Set(
    tags
      .map((tag) => tag.trim())
      .filter((tag) => tag !== '' && tag.toLowerCase() !== 'latest'),
  )
  return Math.max(stableTags.size, 1)
}
