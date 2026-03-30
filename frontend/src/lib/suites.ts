export type RuntimeKind = 'container' | 'mock' | 'script' | 'load' | 'scenario'
export type RuntimeStatus = 'pending' | 'running' | 'healthy' | 'failed'

export interface TopologyNode {
  id: string
  name: string
  kind: RuntimeKind
  dependsOn: string[]
  level: number
}

const NODE_PATTERN = /^([a-zA-Z_][\w]*)\s*=\s*(container|mock|script|load|scenario)\(\s*name="([^"]+)"(?:,\s*after=\[([^\]]*)\])?.*\)$/

export function parseSuiteTopology(suiteStar: string): TopologyNode[] {
  const nodes: TopologyNode[] = []

  for (const rawLine of suiteStar.split('\n')) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) {
      continue
    }

    const match = line.match(NODE_PATTERN)
    if (!match) {
      continue
    }

    const [, , kind, name, rawDeps] = match
    nodes.push({
      id: name,
      name,
      kind: kind as RuntimeKind,
      dependsOn: rawDeps
        ? rawDeps
          .split(',')
          .map((item) => item.trim().replaceAll('"', ''))
          .filter(Boolean)
        : [],
      level: 0,
    })
  }

  const byId = new Map(nodes.map((node) => [node.id, node]))
  const levelCache = new Map<string, number>()

  const resolveLevel = (id: string): number => {
    if (levelCache.has(id)) {
      return levelCache.get(id) ?? 0
    }

    const node = byId.get(id)
    if (!node || node.dependsOn.length === 0) {
      levelCache.set(id, 0)
      return 0
    }

    const level = Math.max(...node.dependsOn.map((dependency) => resolveLevel(dependency))) + 1
    levelCache.set(id, level)
    return level
  }

  return nodes
    .map((node) => ({
      ...node,
      level: resolveLevel(node.id),
    }))
    .sort((left, right) => left.level - right.level || left.name.localeCompare(right.name))
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
