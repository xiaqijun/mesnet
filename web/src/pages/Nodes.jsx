import { useState } from 'react'
import { Link } from 'react-router-dom'
import { usePolling } from '../hooks/usePolling'
import { api } from '../api'
import StatusBadge from '../components/StatusBadge'
import DeployModal from '../components/DeployModal'

export default function Nodes() {
  const { data, loading } = usePolling(() => api.listNodes(), 3000)
  const [deployScript, setDeployScript] = useState('')
  const [showDeploy, setShowDeploy] = useState(false)
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ name: '', subnets: '', backbone: true })

  const handleDeploy = async (id) => {
    const res = await api.getDeployScript(id)
    setDeployScript(res.script)
    setShowDeploy(true)
  }

  const handleAdd = async (e) => {
    e.preventDefault()
    await api.createNode(form)
    setShowAdd(false)
    setForm({ name: '', subnets: '', backbone: true })
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除此节点？关联隧道也会被删除。')) return
    await api.deleteNode(id)
  }

  if (loading) return <div className="text-gray-500">加载中...</div>
  const nodes = data?.nodes || []

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xl font-bold">节点管理</h2>
        <button onClick={() => setShowAdd(!showAdd)} className="px-3 py-1.5 text-xs bg-emerald-600 hover:bg-emerald-500 text-white rounded transition-colors">
          添加节点
        </button>
      </div>

      {showAdd && (
        <form onSubmit={handleAdd} className="bg-gray-900 border border-gray-800 rounded-lg p-4 mb-6 grid grid-cols-2 gap-3">
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="节点名称" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="子网 (逗号分隔)" value={form.subnets} onChange={(e) => setForm({ ...form, subnets: e.target.value })} />
          <label className="flex items-center gap-2 text-xs text-gray-400">
            <input type="checkbox" checked={form.backbone} onChange={(e) => setForm({ ...form, backbone: e.target.checked })} />
            骨干节点
          </label>
          <div className="flex gap-2">
            <button type="submit" className="px-3 py-1.5 text-xs bg-emerald-600 hover:bg-emerald-500 text-white rounded">创建</button>
            <button type="button" onClick={() => setShowAdd(false)} className="px-3 py-1.5 text-xs text-gray-400 hover:text-gray-200">取消</button>
          </div>
        </form>
      )}

      <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-gray-800 text-gray-500">
              <th className="text-left p-3">名称</th>
              <th className="text-left p-3">虚拟 IP</th>
              <th className="text-left p-3">子网</th>
              <th className="text-left p-3">状态</th>
              <th className="text-left p-3">类型</th>
              <th className="text-right p-3">操作</th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((n) => (
              <tr key={n.id} className="border-b border-gray-800/50 hover:bg-gray-800/50">
                <td className="p-3">
                  <Link to={`/nodes/${n.id}`} className="text-emerald-400 hover:underline font-medium">{n.name}</Link>
                </td>
                <td className="p-3 text-gray-400 font-mono">{n.virtual_ip || '-'}</td>
                <td className="p-3 text-gray-400 font-mono text-[10px] max-w-[200px] truncate">{n.subnets || '-'}</td>
                <td className="p-3"><StatusBadge online={n.online} /></td>
                <td className="p-3 text-gray-400">{n.backbone ? '骨干' : '接入'}</td>
                <td className="p-3 text-right space-x-2">
                  <button onClick={() => handleDeploy(n.id)} className="text-emerald-400 hover:underline">部署</button>
                  <button onClick={() => handleDelete(n.id)} className="text-red-400 hover:underline">删除</button>
                </td>
              </tr>
            ))}
            {nodes.length === 0 && (
              <tr><td colSpan={6} className="p-6 text-center text-gray-500">暂无节点，请添加第一个节点</td></tr>
            )}
          </tbody>
        </table>
      </div>

      <DeployModal open={showDeploy} onClose={() => setShowDeploy(false)} script={deployScript} />
    </div>
  )
}
