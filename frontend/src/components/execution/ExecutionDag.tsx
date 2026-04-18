import { useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { FaXmark } from 'react-icons/fa6'
import type { groupTopologyByLevel, RuntimeStatus } from '../../lib/suites'
import './ExecutionDag.css'

type TopologyLevels = ReturnType<typeof groupTopologyByLevel>
type TopologyNode = TopologyLevels[number][number]

const DAG_NODE_W = 180
const DAG_NODE_H = 62
const DAG_COL_GAP = 110
const DAG_ROW_GAP = 20
const DAG_PAD = 40

export function ExecutionDag({
  topology,
  flatTopology,
  statusMap,
  selectedSource,
  onSelectSource,
  onClose,
}: {
  topology: TopologyLevels
  flatTopology: TopologyNode[]
  statusMap: Record<string, RuntimeStatus>
  selectedSource: string
  onSelectSource: (id: string) => void
  onClose: () => void
}) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const [containerSize, setContainerSize] = useState({ w: 0, h: 0 })

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const update = () => setContainerSize({ w: el.clientWidth, h: el.clientHeight })
    update()
    const ro = new ResizeObserver(update)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const maxNodes = Math.max(...topology.map((w) => w.length), 1)
  const totalH = maxNodes * (DAG_NODE_H + DAG_ROW_GAP) - DAG_ROW_GAP

  const positions = useMemo(() => {
    const map = new Map<string, { x: number; y: number }>()
    topology.forEach((wave, wi) => {
      const colH = wave.length * (DAG_NODE_H + DAG_ROW_GAP) - DAG_ROW_GAP
      const startY = (totalH - colH) / 2 + DAG_PAD
      wave.forEach((node, ni) => {
        map.set(node.id, {
          x: wi * (DAG_NODE_W + DAG_COL_GAP) + DAG_PAD,
          y: startY + ni * (DAG_NODE_H + DAG_ROW_GAP),
        })
      })
    })
    return map
  }, [topology, totalH])

  const canvasW = topology.length * (DAG_NODE_W + DAG_COL_GAP) - DAG_COL_GAP + DAG_PAD * 2
  const canvasH = totalH + DAG_PAD * 2

  const scale = containerSize.w > 0 && containerSize.h > 0
    ? Math.min(1, (containerSize.w - DAG_PAD) / canvasW, (containerSize.h - DAG_PAD) / canvasH)
    : 1
  const scaledW = Math.ceil(canvasW * scale)
  const scaledH = Math.ceil(canvasH * scale)

  const edges = useMemo(() => {
    const result: Array<{ fromId: string; toId: string; status: RuntimeStatus }> = []
    flatTopology.forEach((node) => {
      const toPos = positions.get(node.id)
      if (!toPos) return
      node.dependsOn.forEach((depId) => {
        const fromPos = positions.get(depId)
        if (!fromPos) return
        result.push({ fromId: depId, toId: node.id, status: statusMap[depId] })
      })
    })
    return result
  }, [flatTopology, positions, statusMap])

  return createPortal(
    <div className='dag-overlay'>
      <div className='dag-header'>
        <span className='dag-header__title'>Topology Graph</span>
        <div className='dag-header__hint'>Click a node to filter logs</div>
        <button type='button' className='dag-close' onClick={onClose}>
          <FaXmark />
          <span>Close</span>
        </button>
      </div>

      <div className='dag-scroll' ref={scrollRef}>
        <div style={{ width: scaledW, height: scaledH, position: 'relative' }}>
          <div className='dag-canvas' style={{ width: canvasW, height: canvasH, transform: `scale(${scale})`, transformOrigin: 'top left' }}>
            <svg className='dag-svg' width={canvasW} height={canvasH}>
              <defs>
                <marker id='dag-arrow-pending' markerWidth='6' markerHeight='6' refX='5' refY='3' orient='auto'>
                  <path d='M0,0 L6,3 L0,6 Z' fill='#1e3a4e' />
                </marker>
                <marker id='dag-arrow-running' markerWidth='6' markerHeight='6' refX='5' refY='3' orient='auto'>
                  <path d='M0,0 L6,3 L0,6 Z' fill='#0DADEA' />
                </marker>
                <marker id='dag-arrow-healthy' markerWidth='6' markerHeight='6' refX='5' refY='3' orient='auto'>
                  <path d='M0,0 L6,3 L0,6 Z' fill='#18BE94' />
                </marker>
                <marker id='dag-arrow-failed' markerWidth='6' markerHeight='6' refX='5' refY='3' orient='auto'>
                  <path d='M0,0 L6,3 L0,6 Z' fill='#E96D76' />
                </marker>
                <marker id='dag-arrow-skipped' markerWidth='6' markerHeight='6' refX='5' refY='3' orient='auto'>
                  <path d='M0,0 L6,3 L0,6 Z' fill='#7c8b98' />
                </marker>
              </defs>

              {edges.map(({ fromId, toId, status }) => {
                const fp = positions.get(fromId)
                const tp = positions.get(toId)
                if (!fp || !tp) return null
                const sx = fp.x + DAG_NODE_W
                const sy = fp.y + DAG_NODE_H / 2
                const tx = tp.x - 6
                const ty = tp.y + DAG_NODE_H / 2
                const cx = sx + (tx - sx) / 2
                const stroke =
                  status === 'healthy' ? '#18BE94'
                  : status === 'running' ? '#0DADEA'
                  : status === 'failed' ? '#E96D76'
                  : status === 'skipped' ? '#7c8b98'
                  : '#1e3a4e'
                return (
                  <path
                    key={`${fromId}-${toId}`}
                    className={`dag-edge${status === 'running' ? ' dag-edge--running' : ''}`}
                    d={`M ${sx} ${sy} C ${cx} ${sy}, ${cx} ${ty}, ${tx} ${ty}`}
                    fill='none'
                    stroke={stroke}
                    strokeWidth={1.5}
                    strokeOpacity={0.55}
                    markerEnd={`url(#dag-arrow-${status})`}
                  />
                )
              })}
            </svg>

            {flatTopology.map((node) => {
              const pos = positions.get(node.id)
              if (!pos) return null
              const status = statusMap[node.id]
              return (
                <button
                  key={node.id}
                  type='button'
                  className={`dag-node dag-node--${status}${selectedSource === node.id ? ' dag-node--selected' : ''}`}
                  style={{ left: pos.x, top: pos.y, width: DAG_NODE_W, height: DAG_NODE_H }}
                  onClick={() => onSelectSource(node.id)}
                >
                  <span className={`exec-dot exec-dot--${status}`} />
                  <div className='dag-node__text'>
                    <strong>{node.name}</strong>
                    <span>{node.kind}</span>
                  </div>
                </button>
              )
            })}
          </div>
        </div>
      </div>
    </div>,
    document.body,
  )
}
