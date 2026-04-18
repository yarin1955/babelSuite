export interface SimpleYamlParseResult {
  values: Record<string, string>
  errors: string[]
}

export function yamlToJson(yaml: string): string {
  try {
    return JSON.stringify(simpleYamlToObject(yaml), null, 2)
  } catch {
    return '{}'
  }
}

export function simpleYamlToObject(yaml: string): Record<string, unknown> {
  const lines = yaml.split('\n').filter((line) => line.trim() !== '' && !line.trim().startsWith('#'))
  if (lines.length === 0) {
    return {}
  }
  return parseYamlLines(lines, 0, 0)[0]
}

export function objectToYaml(obj: Record<string, unknown>, indent = 0): string {
  const prefix = '  '.repeat(indent)
  const lines: string[] = []
  for (const [key, value] of Object.entries(obj)) {
    if (value === null || value === undefined) {
      lines.push(`${prefix}${key}:`)
    } else if (typeof value === 'object' && !Array.isArray(value)) {
      lines.push(`${prefix}${key}:`)
      lines.push(objectToYaml(value as Record<string, unknown>, indent + 1).trimEnd())
    } else if (Array.isArray(value)) {
      lines.push(`${prefix}${key}:`)
      for (const item of value) {
        lines.push(`${prefix}- ${typeof item === 'object' ? JSON.stringify(item) : item}`)
      }
    } else {
      lines.push(`${prefix}${key}: ${value}`)
    }
  }
  return `${lines.join('\n')}\n`
}

export function parseSimpleYaml(yaml: string): SimpleYamlParseResult {
  const values: Record<string, string> = {}
  const errors: string[] = []
  const stack: string[] = []
  const indents: number[] = []

  for (const rawLine of yaml.split('\n')) {
    if (rawLine.trim() === '' || rawLine.trim().startsWith('#')) {
      continue
    }

    const indent = rawLine.length - rawLine.trimStart().length
    const trimmed = rawLine.trim()
    const separatorIndex = trimmed.indexOf(':')

    if (separatorIndex === -1) {
      errors.push(`Invalid YAML line: "${trimmed}"`)
      continue
    }

    while (indents.length > 0 && indent <= indents[indents.length - 1]) {
      indents.pop()
      stack.pop()
    }

    const key = trimmed.slice(0, separatorIndex).trim()
    const value = trimmed.slice(separatorIndex + 1).trim()

    if (value === '') {
      stack.push(key)
      indents.push(indent)
      continue
    }

    values[[...stack, key].join('.')] = value
  }

  return { values, errors }
}

function parseYamlLines(lines: string[], startIndex: number, baseIndent: number): [Record<string, unknown>, number] {
  const obj: Record<string, unknown> = {}
  let index = startIndex
  while (index < lines.length) {
    const line = lines[index]
    const indent = line.length - line.trimStart().length
    if (indent < baseIndent) {
      break
    }

    const trimmed = line.trim()
    const colonIndex = trimmed.indexOf(':')
    if (colonIndex === -1) {
      index += 1
      continue
    }

    const key = trimmed.slice(0, colonIndex).trim()
    const rest = trimmed.slice(colonIndex + 1).trim()
    if (rest === '') {
      const nextLine = lines[index + 1]
      if (nextLine !== undefined) {
        const nextIndent = nextLine.length - nextLine.trimStart().length
        if (nextIndent > indent) {
          const [child, nextIndex] = parseYamlLines(lines, index + 1, nextIndent)
          obj[key] = child
          index = nextIndex
          continue
        }
      }
      obj[key] = null
    } else {
      obj[key] = rest
    }
    index += 1
  }
  return [obj, index]
}
