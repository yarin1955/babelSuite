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
  const content = active ? artifactContent(active) : null

  return createPortal(
    <div className='exec-dialog-backdrop' onClick={onClose}>
      <div className='exec-dialog exec-dialog--artifacts' onClick={(e) => e.stopPropagation()}>
        <section className='exec-source-preview'>
          <DialogHeader
            eyebrow='Artifacts'
            title={active?.name ?? 'Artifact Results'}
            language={(active?.format ?? 'raw').toUpperCase()}
            copyLabel={copiedId === active?.id ? 'Copied!' : 'Copy'}
            onCopy={content ? () => copyToClipboard(active?.id ?? 'artifact', content) : undefined}
            onClose={onClose}
          />
          {artifacts.length > 1 && (
            <PreviewSwitcher
              items={artifacts.map((artifact) => ({ id: artifact.id, label: artifact.name }))}
              activeId={activeId}
              onSelect={setActiveId}
            />
          )}
          {content ? (
            <CodeLines id={active?.id ?? 'artifact'} content={content} />
          ) : (
            <EmptyPreview>No content available for this artifact.</EmptyPreview>
          )}
        </section>
      </div>
    </div>,
    document.body,
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

function artifactContent(artifact: ExecutionArtifactRecord): string | null {
  if (artifact.content) return artifact.content
  if (artifact.testSummary) {
    const s = artifact.testSummary
    const dur = typeof s.durationSeconds === 'number' ? s.durationSeconds.toFixed(3) : '0'
    const cases = [
      `  <testcase name="${artifact.stepName}" classname="${artifact.stepName}" time="${dur}">${
        s.failures > 0 ? `\n    <failure message="step failed">${artifact.stepName} ended in failed state.</failure>\n  ` : ''
      }</testcase>`,
    ]
    return [
      `<?xml version="1.0" encoding="UTF-8"?>`,
      `<testsuite name="${artifact.stepName}" tests="${s.total}" failures="${s.failures}" errors="${s.errors}" skipped="${s.skipped}" time="${dur}">`,
      ...cases,
      `</testsuite>`,
    ].join('\n')
  }
  return null
}
