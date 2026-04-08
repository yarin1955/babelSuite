export type RuntimeKind = 'container' | 'mock' | 'service' | 'script' | 'load' | 'scenario'
export type RuntimeStatus = 'pending' | 'running' | 'healthy' | 'failed'

export interface TopologyNode {
  id: string
  name: string
  kind: RuntimeKind
  dependsOn: string[]
  level: number
}

const ASSIGNMENT_PATTERN = /^([a-zA-Z_][\w]*)\s*=\s*([a-zA-Z_][\w]*(?:\.[a-zA-Z_][\w]*)?)\((.*)\)$/
const NAME_PATTERN = /(?:^|,)\s*(?:name|name_or_id|id)\s*=\s*"([^"]+)"/
const AFTER_PATTERN = /(?:^|,)\s*after\s*=\s*\[([^\]]*)\]/

export function parseSuiteTopology(suiteStar: string): TopologyNode[] {
  const nodes: TopologyNode[] = []

  for (const rawLine of suiteStar.split('\n')) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) {
      continue
    }

    const node = parseTopologyLine(line)
    if (!node) {
      continue
    }
    nodes.push(node)
  }

  return resolveTopology(nodes)
}

function parseTopologyLine(line: string): TopologyNode | null {
  const match = line.match(ASSIGNMENT_PATTERN)
  if (!match) {
    return null
  }

  const [, , rawCall, rawArgs] = match
  const kind = topologyKind(rawCall)
  if (!kind) {
    return null
  }

  const nameMatch = rawArgs.match(NAME_PATTERN)
  if (!nameMatch) {
    return null
  }

  const afterMatch = rawArgs.match(AFTER_PATTERN)
  return {
    id: nameMatch[1],
    name: nameMatch[1],
    kind,
    dependsOn: afterMatch
      ? afterMatch[1]
          .split(',')
          .map((item) => item.trim().replaceAll('"', ''))
          .filter(Boolean)
          .filter((item, index, items) => items.indexOf(item) === index)
      : [],
    level: 0,
  }
}

function resolveTopology(nodes: TopologyNode[]): TopologyNode[] {
  if (nodes.length === 0) {
    return []
  }

  const byId = new Map<string, TopologyNode>()
  const order = new Map<string, number>()

  for (const [index, node] of nodes.entries()) {
    if (byId.has(node.id)) {
      return []
    }
    byId.set(node.id, node)
    order.set(node.id, index)
  }

  const indegree = new Map<string, number>()
  const dependants = new Map<string, string[]>()
  for (const node of nodes) {
    indegree.set(node.id, node.dependsOn.length)
    for (const dependency of node.dependsOn) {
      if (!byId.has(dependency)) {
        return []
      }
      const children = dependants.get(dependency) ?? []
      children.push(node.id)
      dependants.set(dependency, children)
    }
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
  const call = rawCall.trim()
  switch (call) {
    case 'container':
    case 'container.run':
    case 'container.create':
    case 'container.get':
      return 'container'
    case 'mock':
    case 'mock.serve':
      return 'mock'
    case 'service':
    case 'service.wiremock':
    case 'service.prism':
    case 'service.custom':
      return 'service'
    case 'script':
    case 'script.file':
    case 'script.bash':
    case 'script.sql_migrate':
    case 'script.exec':
      return 'script'
    case 'load':
    case 'load.http':
    case 'load.grpc':
    case 'load.locust':
    case 'load.jmx':
    case 'load.k6':
    case 'scenario':
    case 'scenario.go':
    case 'scenario.python':
    case 'scenario.http':
    case 'suite':
    case 'suite.run':
      if (call.startsWith('load.')) {
        return 'load'
      }
      if (call.startsWith('scenario.')) {
        return 'scenario'
      }
      return call === 'suite' || call === 'suite.run' ? 'container' : call
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
