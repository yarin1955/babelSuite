import type { ChangeEvent, ReactNode } from 'react'

interface AuthFieldProps {
  label: string
  type?: string
  value: string
  autoComplete?: string
  onChange: (event: ChangeEvent<HTMLInputElement>) => void
  trailing?: ReactNode
}

export default function AuthField({
  label,
  type = 'text',
  value,
  autoComplete,
  onChange,
  trailing,
}: AuthFieldProps) {
  return (
    <label className='auth-field'>
      <span className='auth-field__label'>{label}</span>
      <span className='auth-field__control'>
        <input
          className='auth-field__input'
          type={type}
          value={value}
          autoComplete={autoComplete}
          onChange={onChange}
        />
        {trailing}
      </span>
    </label>
  )
}

