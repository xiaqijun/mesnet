import { useEffect, useRef } from 'react'
import * as d3 from 'd3'

export default function TopologyGraph({ nodes, edges }) {
  const ref = useRef(null)

  useEffect(() => {
    if (!nodes || !edges || !ref.current) return
    const svg = d3.select(ref.current)
    svg.selectAll('*').remove()

    const width = ref.current.clientWidth
    const height = ref.current.clientHeight || 500

    const g = svg.attr('viewBox', `0 0 ${width} ${height}`).append('g')

    // Simple force layout
    const nodeMap = {}
    nodes.forEach((n) => { nodeMap[n.id] = n })
    const links = edges.map((e) => ({ source: nodeMap[e.left_id], target: nodeMap[e.right_id], ...e }))

    const simulation = d3.forceSimulation(nodes)
      .force('link', d3.forceLink(links).id((d) => d.id).distance(150))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(width / 2, height / 2))

    const link = g.append('g').selectAll('line')
      .data(links).join('line')
      .attr('stroke', (d) => d.status === 'up' ? '#34d399' : '#374151')
      .attr('stroke-width', (d) => Math.max(1, Math.min(6, (d.rx_bytes + d.tx_bytes) / 100000)))
      .attr('stroke-opacity', 0.6)

    const node = g.append('g').selectAll('g')
      .data(nodes).join('g')

    node.append('circle')
      .attr('r', 18)
      .attr('fill', (d) => d.online ? '#064e3b' : '#1f2937')
      .attr('stroke', (d) => d.online ? '#34d399' : '#4b5563')
      .attr('stroke-width', 2)

    node.append('text')
      .text((d) => d.name)
      .attr('text-anchor', 'middle')
      .attr('dy', 35)
      .attr('fill', '#9ca3af')
      .attr('font-size', 11)

    simulation.on('tick', () => {
      link.attr('x1', (d) => d.source.x).attr('y1', (d) => d.source.y)
          .attr('x2', (d) => d.target.x).attr('y2', (d) => d.target.y)
      node.attr('transform', (d) => `translate(${d.x},${d.y})`)
    })

    return () => simulation.stop()
  }, [nodes, edges])

  return <svg ref={ref} className="w-full h-[500px] bg-gray-900 rounded-lg border border-gray-800" />
}
