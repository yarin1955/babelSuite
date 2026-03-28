import type { ReactNode } from 'react'
import { createPortal } from 'react-dom'
import './SlidingPanel.css'

interface SlidingPanelProps {
  isOpen: boolean
  onClose: () => void
  header: ReactNode
  children: ReactNode
  width?: string
}

export default function SlidingPanel({ isOpen, onClose, header, children, width = '600px' }: SlidingPanelProps) {
  return createPortal(
    <>
      <div
        className={`sliding-panel-overlay${isOpen ? ' sliding-panel-overlay--open' : ''}`}
        onClick={onClose}
      />
      <div
        className={`sliding-panel${isOpen ? ' sliding-panel--open' : ''}`}
        style={{ width }}
      >
        <div className='sliding-panel__header'>
          {header}
        </div>
        <div className='sliding-panel__body'>
          {children}
        </div>
      </div>
    </>,
    document.body,
  )
}
