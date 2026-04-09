import type { ReactNode } from 'react'
import { FaChevronDown } from 'react-icons/fa6'

interface CollapsibleSectionProps {
  title: string
  open: boolean
  onToggle: () => void
  children: ReactNode
}

export function CollapsibleSection({ title, open, onToggle, children }: CollapsibleSectionProps) {
  return (
    <section className={`profiles-section${open ? ' profiles-section--open' : ''}`}>
      <button type='button' className='profiles-section__header' onClick={onToggle}>
        <span>{title}</span>
        <FaChevronDown />
      </button>
      <div className='profiles-section__body'>{children}</div>
    </section>
  )
}
