interface HeaderListProps {
  headers: Array<{ name: string; value: string }>
}

export function HeaderList({ headers }: HeaderListProps) {
  if (!headers.length) return null
  return (
    <div className='suite-header-list'>
      {headers.map((h) => (
        <div key={`${h.name}-${h.value}`} className='suite-header-list__row'>
          <span>{h.name}</span>
          <strong>{h.value}</strong>
        </div>
      ))}
    </div>
  )
}
