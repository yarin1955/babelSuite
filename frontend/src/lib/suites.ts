export type RuntimeKind = 'mock' | 'service' | 'task' | 'test' | 'traffic' | 'suite'
export type RuntimeStatus = 'pending' | 'running' | 'healthy' | 'failed' | 'skipped'

export interface ArtifactExport {
  path: string
  name?: string
  on?: string
  format?: string
}

export interface TopologyNode {
  id: string
  name: string
  kind: string
  variant?: string
  dependsOn: string[]
  resetMocks?: string[]
  artifactExports?: ArtifactExport[]
  level: number
}

interface ParsedTopologyNode extends TopologyNode {
  assignment: string
}

export function parseSuiteTopology(suiteStar: string): TopologyNode[] {
  const nodes: ParsedTopologyNode[] = []

  for (const line of topologyStatements(suiteStar)) {
    const node = parseTopologyLine(line)
    if (!node) {
      continue
    }
    nodes.push(node)
  }

  return resolveTopology(nodes)
}

function parseTopologyLine(line: string): ParsedTopologyNode | null {
  const assignment = parseTopologyAssignment(line)
  if (!assignment) {
    return null
  }

  const invocation = parseTopologyInvocation(assignment.expression)
  if (!invocation) {
    return null
  }

  const canonicalCall = canonicalRuntimeCall(invocation.call)
  const kind = topologyKind(canonicalCall)
  if (!kind) {
    return null
  }

  const name = topologyNamedStringArgument(invocation.args, 'name', 'name_or_id', 'id') ?? assignment.name
  const afterDependencies = topologyDependencies(invocation.args, 'after')
  const failureDependencies = topologyDependencies(invocation.args, 'on_failure')
  const resetMocks = topologyDependencies(invocation.args, 'reset_mocks')
  return {
    id: name,
    name,
    kind,
    variant: canonicalCall,
    dependsOn: Array.from(new Set([...afterDependencies, ...failureDependencies])),
    resetMocks: resetMocks.length > 0 ? resetMocks : undefined,
    artifactExports: invocation.artifactExports.length > 0 ? invocation.artifactExports : undefined,
    level: 0,
    assignment: assignment.name,
  }
}

function parseTopologyAssignment(line: string): { name: string; expression: string } | null {
  const separator = findTopologyAssignmentSeparator(line)
  if (separator <= 0) {
    return null
  }

  const name = line.slice(0, separator).trim()
  const expression = line.slice(separator + 1).trim()
  if (!isTopologyIdentifier(name) || !expression) {
    return null
  }
  return { name, expression }
}

function findTopologyAssignmentSeparator(line: string): number {
  let depthParen = 0
  let depthBracket = 0
  let depthBrace = 0
  let inString = false
  let escaped = false

  for (let index = 0; index < line.length; index += 1) {
    const char = line[index]
    if (inString) {
      if (escaped) {
        escaped = false
        continue
      }
      if (char === '\\') {
        escaped = true
        continue
      }
      if (char === '"') {
        inString = false
      }
      continue
    }

    if (char === '"') {
      inString = true
      continue
    }
    if (char === '(') {
      depthParen += 1
      continue
    }
    if (char === ')') {
      depthParen = Math.max(0, depthParen - 1)
      continue
    }
    if (char === '[') {
      depthBracket += 1
      continue
    }
    if (char === ']') {
      depthBracket = Math.max(0, depthBracket - 1)
      continue
    }
    if (char === '{') {
      depthBrace += 1
      continue
    }
    if (char === '}') {
      depthBrace = Math.max(0, depthBrace - 1)
      continue
    }
    if (char === '=' && depthParen === 0 && depthBracket === 0 && depthBrace === 0) {
      return index
    }
  }

  return -1
}

function topologyStatements(suiteStar: string): string[] {
  const statements: string[] = []
  let current = ''

  const flush = () => {
    const statement = current.trim()
    if (statement) {
      statements.push(statement)
    }
    current = ''
  }

  for (const rawLine of suiteStar.split('\n')) {
    let line = rawLine.trim()
    if (!line || line.startsWith('#')) {
      continue
    }

    const continued = line.endsWith('\\')
    if (continued) {
      line = line.slice(0, -1).trim()
    }
    if (!line) {
      continue
    }

    current = current ? `${current} ${line}` : line
    if (!continued) {
      flush()
    }
  }

  flush()
  return statements
}

function parseTopologyInvocation(
  expression: string,
): { call: string; args: string; artifactExports: ArtifactExport[] } | null {
  const openIndex = expression.indexOf('(')
  if (openIndex <= 0) {
    return null
  }

  const call = expression.slice(0, openIndex).trim()
  if (!isTopologyCallPath(call)) {
    return null
  }

  const base = readCallArguments(expression, openIndex)
  if (!base) {
    return null
  }

  const invocation = {
    call,
    args: base.args,
    artifactExports: [] as ArtifactExport[],
  }

  let rest = expression.slice(base.nextIndex).trim()
  while (rest) {
    if (!rest.startsWith('.')) {
      return null
    }

    rest = rest.slice(1).trim()
    const methodOpenIndex = rest.indexOf('(')
    if (methodOpenIndex <= 0) {
      return null
    }

    const method = rest.slice(0, methodOpenIndex).trim()
    const chained = readCallArguments(rest, methodOpenIndex)
    if (!chained || method !== 'export') {
      return null
    }

    const artifactExport = parseArtifactExport(chained.args)
    if (!artifactExport) {
      return null
    }
    invocation.artifactExports.push(artifactExport)
    rest = rest.slice(chained.nextIndex).trim()
  }

  return invocation
}

function isTopologyCallPath(value: string): boolean {
  if (!value) {
    return false
  }
  return value.split('.').every((part) => isTopologyIdentifier(part))
}

function canonicalRuntimeCall(call: string): string {
  const trimmed = call.trim()
  if (trimmed.startsWith('load.')) {
    return `traffic.${trimmed.slice('load.'.length)}`
  }
  switch (trimmed) {
    case 'mock.serve':
      return 'service.mock'
    default:
      return trimmed
  }
}

function readCallArguments(expression: string, openIndex: number): { args: string; nextIndex: number } | null {
  if (openIndex < 0 || openIndex >= expression.length || expression[openIndex] !== '(') {
    return null
  }

  let depth = 1
  let inString = false
  let escaped = false
  for (let index = openIndex + 1; index < expression.length; index += 1) {
    const char = expression[index]
    if (inString) {
      if (escaped) {
        escaped = false
        continue
      }
      if (char === '\\') {
        escaped = true
        continue
      }
      if (char === '"') {
        inString = false
      }
      continue
    }

    if (char === '"') {
      inString = true
      continue
    }
    if (char === '(') {
      depth += 1
      continue
    }
    if (char === ')') {
      depth -= 1
      if (depth === 0) {
        return {
          args: expression.slice(openIndex + 1, index),
          nextIndex: index + 1,
        }
      }
    }
  }

  return null
}

function parseArtifactExport(argumentText: string): ArtifactExport | null {
  const artifactExport: ArtifactExport = { path: '', on: 'success' }

  for (const [index, rawPart] of splitTopLevel(argumentText).entries()) {
    const part = rawPart.trim()
    if (!part) {
      continue
    }

    if (part.includes('=')) {
      const separatorIndex = part.indexOf('=')
      const key = part.slice(0, separatorIndex).trim()
      const value = unquoteTopologyString(part.slice(separatorIndex + 1).trim())
      if (value == null) {
        return null
      }
      if (key === 'path') {
        artifactExport.path = value
        continue
      }
      if (key === 'name') {
        artifactExport.name = value
        continue
      }
      if (key === 'on') {
        artifactExport.on = value
        continue
      }
      if (key === 'format') {
        artifactExport.format = value.toLowerCase()
        continue
      }
      return null
    }

    if (index > 0 || artifactExport.path) {
      return null
    }
    const value = unquoteTopologyString(part)
    if (value == null) {
      return null
    }
    artifactExport.path = value
  }

  if (!artifactExport.path) {
    return null
  }
  if (!['success', 'failure', 'always'].includes(artifactExport.on ?? 'success')) {
    return null
  }
  return artifactExport
}

function topologyNamedStringArgument(argumentText: string, ...keys: string[]): string | null {
  const rawValue = topologyNamedArgument(argumentText, ...keys)
  if (!rawValue) {
    return null
  }
  return unquoteTopologyString(rawValue)
}

function topologyNamedArgument(argumentText: string, ...keys: string[]): string | null {
  const keySet = new Set(keys.map((key) => key.trim()))
  for (const part of splitTopLevel(argumentText)) {
    const separatorIndex = part.indexOf('=')
    if (separatorIndex <= 0) {
      continue
    }
    const key = part.slice(0, separatorIndex).trim()
    if (!keySet.has(key)) {
      continue
    }
    return part.slice(separatorIndex + 1).trim()
  }
  return null
}

function topologyDependencies(argumentText: string, ...keys: string[]): string[] {
  const keyList = keys.length > 0 ? keys : ['after']
  const values = keyList
    .map((key) => topologyNamedArgument(argumentText, key))
    .filter((value): value is string => Boolean(value))
  if (values.length === 0) {
    return []
  }

  const dependencies = values.flatMap((rawValue) => {
  if (!rawValue) {
      return []
  }

  const trimmed = rawValue.trim()
  if (!trimmed.startsWith('[') || !trimmed.endsWith(']')) {
      return []
  }

    return splitTopLevel(trimmed.slice(1, -1))
    .map((item) => {
      const trimmedItem = item.trim()
      if (!trimmedItem) {
        return ''
      }
      if (trimmedItem.startsWith('"')) {
        return unquoteTopologyString(trimmedItem) ?? ''
      }
      return trimmedItem
    })
    .filter(Boolean)
  })

  return dependencies.filter((item, index, items) => items.indexOf(item) === index)
}

function splitTopLevel(argumentText: string): string[] {
  const parts: string[] = []
  let current = ''
  let depthParen = 0
  let depthBracket = 0
  let depthBrace = 0
  let inString = false
  let escaped = false

  const flush = () => {
    const part = current.trim()
    if (part) {
      parts.push(part)
    }
    current = ''
  }

  for (const char of argumentText) {
    if (inString) {
      current += char
      if (escaped) {
        escaped = false
        continue
      }
      if (char === '\\') {
        escaped = true
        continue
      }
      if (char === '"') {
        inString = false
      }
      continue
    }

    if (char === '"') {
      inString = true
      current += char
      continue
    }
    if (char === '(') {
      depthParen += 1
      current += char
      continue
    }
    if (char === ')') {
      depthParen = Math.max(0, depthParen - 1)
      current += char
      continue
    }
    if (char === '[') {
      depthBracket += 1
      current += char
      continue
    }
    if (char === ']') {
      depthBracket = Math.max(0, depthBracket - 1)
      current += char
      continue
    }
    if (char === '{') {
      depthBrace += 1
      current += char
      continue
    }
    if (char === '}') {
      depthBrace = Math.max(0, depthBrace - 1)
      current += char
      continue
    }
    if (char === ',' && depthParen === 0 && depthBracket === 0 && depthBrace === 0) {
      flush()
      continue
    }
    current += char
  }

  flush()
  return parts
}

function unquoteTopologyString(value: string): string | null {
  const trimmed = value.trim()
  if (!trimmed.startsWith('"') || !trimmed.endsWith('"')) {
    return null
  }

  try {
    return JSON.parse(trimmed) as string
  } catch {
    return null
  }
}

function isTopologyIdentifier(value: string): boolean {
  if (!value) {
    return false
  }

  for (let index = 0; index < value.length; index += 1) {
    const char = value[index]
    const isAlpha = (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
    const isDigit = char >= '0' && char <= '9'
    if (index === 0) {
      if (!(isAlpha || char === '_')) {
        return false
      }
      continue
    }
    if (!(isAlpha || isDigit || char === '_')) {
      return false
    }
  }

  return true
}

function resolveTopology(nodes: ParsedTopologyNode[]): TopologyNode[] {
  if (nodes.length === 0) {
    return []
  }

  const byId = new Map<string, ParsedTopologyNode>()
  const order = new Map<string, number>()
  const assignmentToID = new Map<string, string>()

  for (const [index, node] of nodes.entries()) {
    if (byId.has(node.id)) {
      return []
    }
    byId.set(node.id, node)
    order.set(node.id, index)
    assignmentToID.set(node.assignment, node.id)
  }

  const indegree = new Map<string, number>()
  const dependants = new Map<string, string[]>()
  for (const node of nodes) {
    const normalizedDependencies = node.dependsOn.map((dependency) => assignmentToID.get(dependency) ?? dependency)
    indegree.set(node.id, normalizedDependencies.length)
    for (const dependency of normalizedDependencies) {
      if (!byId.has(dependency)) {
        return []
      }
      const children = dependants.get(dependency) ?? []
      children.push(node.id)
      dependants.set(dependency, children)
    }
    node.dependsOn = normalizedDependencies
  }

  let ready = nodes.filter((node) => (indegree.get(node.id) ?? 0) === 0).map((node) => node.id)
  ready.sort((left, right) => (order.get(left) ?? 0) - (order.get(right) ?? 0))

  const resolved: TopologyNode[] = []
  let level = 0
  while (ready.length > 0) {
    const current = [...ready]
    const nextReady: string[] = []

    for (const id of current) {
      const node = byId.get(id)
      if (!node) {
        continue
      }
      resolved.push({
        ...node,
        level,
      })

      for (const dependant of dependants.get(id) ?? []) {
        const nextDegree = (indegree.get(dependant) ?? 0) - 1
        indegree.set(dependant, nextDegree)
        if (nextDegree === 0) {
          nextReady.push(dependant)
        }
      }
    }

    nextReady.sort((left, right) => (order.get(left) ?? 0) - (order.get(right) ?? 0))
    ready = nextReady
    level++
  }

  if (resolved.length !== nodes.length) {
    return []
  }

  return resolved
}

function topologyKind(rawCall: string): RuntimeKind | null {
  const call = canonicalRuntimeCall(rawCall.trim())
  switch (call) {
    case 'service.mock':
      return 'mock'
    case 'service.run':
    case 'service.wiremock':
    case 'service.prism':
    case 'service.custom':
      return 'service'
    case 'task.run':
      return 'task'
    case 'test.run':
      return 'test'
    case 'traffic.smoke':
    case 'traffic.baseline':
    case 'traffic.stress':
    case 'traffic.spike':
    case 'traffic.soak':
    case 'traffic.scalability':
    case 'traffic.step':
    case 'traffic.wave':
    case 'traffic.staged':
    case 'traffic.constant_throughput':
    case 'traffic.constant_pacing':
    case 'traffic.open_model':
      return 'traffic'
    case 'suite.run':
      return 'suite'
    default:
      return null
  }
}

export function groupTopologyByLevel(topology: TopologyNode[]) {
  const levels = Array.from(new Set(topology.map((node) => node.level))).sort((left, right) => left - right)
  return levels.map((level) => topology.filter((node) => node.level === level))
}

export function deriveRuntimeStatus(
  topology: TopologyNode[],
  events: Array<{ source: string; status: RuntimeStatus }>,
) {
  const statuses = Object.fromEntries(topology.map((node) => [node.id, 'pending'])) as Record<string, RuntimeStatus>

  for (const event of events) {
    statuses[event.source] = event.status
  }

  return statuses
}
