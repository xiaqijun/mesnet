import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge'
import TrafficChart from '../components/TrafficChart'

export default function TunnelDetail() {
  const { id } = useParams()
  const { data, loading } = usePolling(() => api.getTunnel(id), 3000)
  const { data: statsData } = usePolling(() => api.getTunnelStats(id).catch(() => ({})), 3000)
  const [range, setRange] = useState('1h')
  const { data: historyData } = usePolling(() => api.getTunnelHistory(id, range).catch(() => ({})), 5000)

  if (loading) return <div className="text-gray-500">加载中...</div>
  const tunnel = data?.tunnel
  if (!tunnel) return <div className="text-gray-500">隧道不存在</div>

  return (
    <div>
      <Link to="/tunnels" className="text-xs text-gray-500 hover:text-gray-300">&larr; 返回隧道列表</Link>
      <div className="flex items-center gap-3 mt-2 mb-6">
        <h2 className="text-xl font-bold">{tunnel.name}</h2>
        <StatusBadge online={tunnel.status === 'up'} label={tunnel.status === 'up' ? 'UP' : 'DOWN'} />
      </div>

      <div className="grid grid-cols-4 gap-4 mb-6">
        {[
          { label: '左端节点', value: tunnel.left_node?.name || `#${tunnel.left_node_id}` },
          { label: '右端节点', value: tunnel.right_node?.name || `#${tunnel.right_node_id}` },
          { label: '左端子网', value: tunnel.left_subnet || '-' },
          { label: '右端子网', value: tunnel.right_subnet || '-' },
        ].map((c) => (
          <div key={c.label} className="bg-gray-900 border border-gray-800 rounded-lg p-3">
            <p className="text-sm font-bold text-gray-200">{c.value}</p>
            <p className="text-xs text-gray-500">{c.label}</p>
          </div>
        ))}
      </div>

      {statsData && (
        <div className="grid grid-cols-3 gap-4 mb-6">
          {[
            { label: 'RX', value: statsData.rx_bytes, color: 'text-emerald-400' },
            { label: 'TX', value: statsData.tx_bytes, color: 'text-blue-400' },
            { label: '延迟', value: statsData.latency_ms ? statsData.latency_ms.toFixed(1) + ' ms' : '-', color: 'text-amber-400' },
          ].map((c) => (
            <div key={c.label} className="bg-gray-900 border border-gray-800 rounded-lg p-3">
              <p className={`text-sm font-bold ${c.color}`}>
                {typeof c.value === 'number' ? (c.value >= 1e6 ? (c.value / 1e6).toFixed(1) + ' MB' : c.value + ' B') : c.value}
              </p>
              <p className="text-xs text-gray-500">{c.label}</p>
            </div>
          ))}
        </div>
      )}

      <div className="bg-gray-900 border border-gray-800 rounded-lg p-4 mb-4">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-bold text-gray-300">流量历史</h3>
          <div className="flex gap-1">
            {['1h', '6h', '24h', '7d'].map((r) => (
              <button key={r} onClick={() => setRange(r)} className={`px-2 py-0.5 text-[10px] rounded ${range === r ? 'bg-emerald-600/20 text-emerald-400' : 'text-gray-500 hover:text-gray-300'}`}>
                {r}
              </button>
            ))}
          </div>
        </div>
        <TrafficChart data={historyData?.snapshots || []} />
      </div>
    </div>
  )
}
