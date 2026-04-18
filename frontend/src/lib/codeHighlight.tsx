import type { ReactNode } from 'react'

interface TokenClasses {
  base: string
  keyword: string
  string: string
  module: string
  comment: string
}

export const ciTokenClasses: TokenClasses = {
  base: 'ci-tok',
  keyword: 'ci-tok--kw',
  string: 'ci-tok--str',
  module: 'ci-tok--mod',
  comment: 'ci-tok--cmt',
}

export const suiteTokenClasses: TokenClasses = {
  base: 'suite-token',
  keyword: 'suite-token--keyword',
  string: 'suite-token--string',
  module: 'suite-token--module',
  comment: 'suite-token--comment',
}

export function renderStarlarkLine(line: string, classes = ciTokenClasses): ReactNode[] {
  return renderLineTokens(
    line,
    /"[^"]*"|\b(load|service|task|test|traffic|suite|container|mock|script|scenario)\b|@[a-zA-Z0-9/_-]+/g,
    classes,
  )
}

export function renderSourceLine(line: string, language: string, classes = ciTokenClasses): ReactNode[] {
  const trimmedLanguage = language.trim().toLowerCase()
  if (trimmedLanguage === 'yaml' || trimmedLanguage === 'python' || trimmedLanguage === 'bash' || trimmedLanguage === 'rego') {
    return renderLineTokens(
      line,
      /"[^"]*"|'[^']*'|\b(message|service|rpc|package|import|const|let|type|interface|export|default|allow|if|true|false|null)\b|@[a-zA-Z0-9/_-]+/g,
      classes,
    )
  }

  return highlightCodeTokens(line, classes)
}

function renderLineTokens(line: string, pattern: RegExp, classes: TokenClasses) {
  const commentIndex = line.indexOf('#')
  const code = commentIndex >= 0 ? line.slice(0, commentIndex) : line
  const comment = commentIndex >= 0 ? line.slice(commentIndex) : ''
  const fragments = highlightCodeTokens(code, classes, pattern)
  if (comment) {
    fragments.push(<span key={`comment-${comment}`} className={tokenClass(classes, 'comment')}>{comment}</span>)
  }
  return fragments
}

function highlightCodeTokens(
  line: string,
  classes: TokenClasses,
  pattern = /"[^"]*"|'[^']*'|\b(message|service|rpc|package|import|const|let|type|interface|export|default|allow|if|true|false|null)\b|@[a-zA-Z0-9/_-]+/g,
) {
  const fragments: ReactNode[] = []
  let cursor = 0

  for (const match of line.matchAll(pattern)) {
    const value = match[0]
    const start = match.index ?? 0
    if (start > cursor) {
      fragments.push(line.slice(cursor, start))
    }
    fragments.push(
      <span key={`${start}-${value}`} className={tokenClass(classes, tokenKind(value))}>
        {value}
      </span>,
    )
    cursor = start + value.length
  }

  if (cursor < line.length) {
    fragments.push(line.slice(cursor))
  }
  return fragments
}

function tokenKind(value: string): 'keyword' | 'string' | 'module' {
  if (value.startsWith('"') || value.startsWith("'")) {
    return 'string'
  }
  if (value.startsWith('@')) {
    return 'module'
  }
  return 'keyword'
}

function tokenClass(classes: TokenClasses, kind: keyof Omit<TokenClasses, 'base'>) {
  return `${classes.base} ${classes[kind]}`
}
