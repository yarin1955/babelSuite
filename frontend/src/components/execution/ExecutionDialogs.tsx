import { useState } from 'react'
import { createPortal } from 'react-dom'
import { FaCopy, FaXmark } from 'react-icons/fa6'
import { useClipboardFeedback } from '../../hooks/useClipboardFeedback'
import type { ExecutionArtifactRecord } from '../../lib/api'
import './ExecutionDialogs.css'

export interface MockPreview {
  id: string
  label: string
  language: string
  content: string
}

interface MockDialogProps {
  mockPreviews: MockPreview[]
  activeMockPreview: MockPreview | undefined
  onSelectMockPreview: (id: string) => void
  onCopy: () => Promise<void>
  onClose: () => void
}

export function MockDialog({
  mockPreviews,
  activeMockPreview,
  onSelectMockPreview,
  onCopy,
  onClose,
}: MockDialogProps) {
  return createPortal(
    <div className='exec-dialog-backdrop' onClick={onClose}>
      <div className='exec-dialog exec-dialog--mock' onClick={(e) => e.stopPropagation()}>
        <section className='exec-source-preview'>
          <DialogHeader
            eyebrow='Generated Mock Data'
            title={activeMockPreview?.label ?? 'Mock Responses'}
            language={activeMockPreview?.language}
            onCopy={activeMockPreview ? onCopy : undefined}
            onClose={onClose}
          />
          {mockPreviews.length > 1 && (
            <PreviewSwitcher
              items={mockPreviews}
              activeId={activeMockPreview?.id}
              onSelect={onSelectMockPreview}
            />
          )}
          {activeMockPreview ? (
            <CodeLines id={activeMockPreview.id} content={activeMockPreview.content} />
          ) : (
            <EmptyPreview>Waiting for mock data to become available for this suite.</EmptyPreview>
          )}
        </section>
      </div>
    </div>,
    document.body,
  )
}

interface ArtifactsDialogProps {
  artifacts: ExecutionArtifactRecord[]
  onClose: () => void
}

export function ArtifactsDialog({ artifacts, onClose }: ArtifactsDialogProps) {
  const [activeId, setActiveId] = useState(artifacts[0]?.id ?? '')
  const { copiedId, copyToClipboard } = useClipboardFeedback(1600)
  const active = artifacts.find((a) => a.id === activeId) ?? artifacts[0]
  const rawContent = active?.content ?? null

  return createPortal(
    <div className='exec-dialog-backdrop' onClick={onClose}>
      <div className='exec-dialog exec-dialog--artifacts' onClick={(e) => e.stopPropagation()}>
        <section className='exec-source-preview'>
          <DialogHeader
            eyebrow='Artifacts'
            title={active?.name ?? 'Artifact Results'}
            language={(active?.format ?? 'raw').toUpperCase()}
            copyLabel={copiedId === active?.id ? 'Copied!' : 'Copy'}
            onCopy={rawContent ? () => copyToClipboard(active?.id ?? 'artifact', rawContent) : undefined}
            onClose={onClose}
          />
          {artifacts.length > 1 && (
            <PreviewSwitcher
              items={artifacts.map((artifact) => ({ id: artifact.id, label: artifact.name }))}
              activeId={activeId}
              onSelect={setActiveId}
            />
          )}
          {active?.format === 'junit' && active.testSummary ? (
            <JUnitPanel summary={active.testSummary} />
          ) : active?.format === 'cobertura' && active.coverageSummary ? (
            <CoveragePanel summary={active.coverageSummary} />
          ) : rawContent ? (
            <CodeLines id={active?.id ?? 'artifact'} content={rawContent} />
          ) : (
            <EmptyPreview>No content available for this artifact.</EmptyPreview>
          )}
        </section>
      </div>
    </div>,
    document.body,
  )
}

function JUnitPanel({ summary }: { summary: NonNullable<ExecutionArtifactRecord['testSummary']> }) {
  const dur = typeof summary.durationSeconds === 'number' ? summary.durationSeconds.toFixed(2) + 's' : null
  const allPassed = summary.failures === 0 && summary.errors === 0
  return (
    <div className='exec-source-preview__body'>
      <div className='artifact-panel artifact-junit'>
        <div className='artifact-junit__stats'>
          <div className='artifact-stat'>
            <div className='artifact-stat__label'>Total</div>
            <div className='artifact-stat__value'>{summary.total}</div>
          </div>
          <div className={`artifact-stat${allPassed ? ' artifact-stat--pass' : ''}`}>
            <div className='artifact-stat__label'>Passed</div>
            <div className='artifact-stat__value'>{summary.passed}</div>
          </div>
          <div className={`artifact-stat${summary.failures > 0 ? ' artifact-stat--fail' : ''}`}>
            <div className='artifact-stat__label'>Failures</div>
            <div className='artifact-stat__value'>{summary.failures}</div>
          </div>
          <div className={`artifact-stat${summary.errors > 0 ? ' artifact-stat--fail' : ''}`}>
            <div className='artifact-stat__label'>Errors</div>
            <div className='artifact-stat__value'>{summary.errors}</div>
          </div>
          <div className='artifact-stat artifact-stat--skip'>
            <div className='artifact-stat__label'>Skipped</div>
            <div className='artifact-stat__value'>{summary.skipped}</div>
          </div>
          {dur && (
            <div className='artifact-stat'>
              <div className='artifact-stat__label'>Duration</div>
              <div className='artifact-stat__value artifact-stat__value--sm'>{dur}</div>
            </div>
          )}
        </div>
        <div className={`artifact-junit__verdict${allPassed ? ' artifact-junit__verdict--pass' : ' artifact-junit__verdict--fail'}`}>
          {allPassed ? 'All tests passed' : `${summary.failures + summary.errors} test${summary.failures + summary.errors !== 1 ? 's' : ''} did not pass`}
        </div>
      </div>
    </div>
  )
}

function CoveragePanel({ summary }: { summary: NonNullable<ExecutionArtifactRecord['coverageSummary']> }) {
  const linePct = Math.round((summary.lineRate ?? 0) * 100)
  const branchPct = Math.round((summary.branchRate ?? 0) * 100)
  return (
    <div className='exec-source-preview__body'>
      <div className='artifact-panel artifact-coverage'>
        <CoverageBar
          label='Line Coverage'
          pct={linePct}
          covered={summary.linesCovered}
          total={summary.linesValid}
        />
        <CoverageBar
          label='Branch Coverage'
          pct={branchPct}
          covered={summary.branchesCovered}
          total={summary.branchesValid}
        />
      </div>
    </div>
  )
}

function CoverageBar({
  label,
  pct,
  covered,
  total,
}: {
  label: string
  pct: number
  covered: number | undefined
  total: number | undefined
}) {
  const detail = covered != null && total != null && total > 0 ? `${covered} / ${total}` : null
  return (
    <div className='artifact-coverage__row'>
      <div className='artifact-coverage__label-row'>
        <span>{label}</span>
        <span className='artifact-coverage__pct'>
          {pct}%{detail ? ` — ${detail}` : ''}
        </span>
      </div>
      <div className='artifact-coverage__bar'>
        <div
          className='artifact-coverage__fill'
          style={{ width: `${Math.min(pct, 100)}%`, background: pct >= 80 ? '#18be94' : pct >= 50 ? '#f0a830' : '#e85a7e' }}
        />
      </div>
    </div>
  )
}

function DialogHeader({
  eyebrow,
  title,
  language,
  copyLabel = 'Copy',
  onCopy,
  onClose,
}: {
  eyebrow: string
  title: string
  language?: string
  copyLabel?: string
  onCopy?: () => void | Promise<void>
  onClose: () => void
}) {
  return (
    <div className='exec-source-preview__header'>
      <div>
        <p className='exec-source-preview__eyebrow'>{eyebrow}</p>
        <h3>{title}</h3>
      </div>
      <div className='exec-source-preview__header-right'>
        {onCopy && (
          <div className='exec-source-preview__actions'>
            {language && <span className='exec-source-preview__language'>{language}</span>}
            <button type='button' className='exec-source-preview__copy' onClick={() => void onCopy()}>
              <FaCopy />
              <span>{copyLabel}</span>
            </button>
          </div>
        )}
        <button type='button' className='exec-dialog-close' onClick={onClose}>
          <FaXmark />
          <span>Close</span>
        </button>
      </div>
    </div>
  )
}

function PreviewSwitcher({
  items,
  activeId,
  onSelect,
}: {
  items: Array<{ id: string; label: string }>
  activeId: string | undefined
  onSelect: (id: string) => void
}) {
  return (
    <div className='exec-source-preview__switcher'>
      {items.map((item) => (
        <button
          key={item.id}
          type='button'
          className={`exec-source-preview__chip${item.id === activeId ? ' exec-source-preview__chip--active' : ''}`}
          onClick={() => onSelect(item.id)}
        >
          {item.label}
        </button>
      ))}
    </div>
  )
}

function CodeLines({ id, content }: { id: string; content: string }) {
  return (
    <div className='exec-source-preview__body'>
      {content.split('\n').map((line, index) => (
        <div key={`${id}-${index + 1}`} className='exec-source-preview__line'>
          <span className='exec-source-preview__line-number'>{String(index + 1).padStart(3, ' ')}</span>
          <code className='exec-source-preview__line-content'>{line || ' '}</code>
        </div>
      ))}
    </div>
  )
}

function EmptyPreview({ children }: { children: string }) {
  return <div className='exec-source-preview__empty'>{children}</div>
}

