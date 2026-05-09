import { usePolling } from '../hooks/usePolling'
import { api } from '../api'
import TopologyGraph from '../components/TopologyGraph'

export default function Topology() {
  const { data, loading } = usePolling(() => api.getTopology(), 3000)

  if (loading) return <div className="text-gray-500">加载中...</div>

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">网络拓扑</h2>
      <p className="text-xs text-gray-500 mb-4">节点间边宽反映流量大小，绿色=在线，灰色=离线</p>
      <TopologyGraph nodes={data?.nodes || []} edges={data?.edges || []} />
    </div>
  )
}
