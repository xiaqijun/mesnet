import { useState, useEffect } from 'react'
import { usePolling } from '../hooks/usePolling'
import StatusBadge from '../components/StatusBadge'
import DeployModal from '../components/DeployModal'

export default function Servers() {
  const { data, loading } = usePolling(() => fetchServers(), 3000)
  const [deployScript, setDeployScript] = useState('')
  const [showDeploy, setShowDeploy] = useState(false)
  const [sshResult, setSshResult] = useState(null)
  const [latestVer, setLatestVer] = useState('')
  useEffect(() => { fetch('/api/agents/versions').then(r => r.json()).then(d => setLatestVer(d.server_version || '')) }, [])
  const [showAddCloud, setShowAddCloud] = useState(false)
  const [showAddLeaf, setShowAddLeaf] = useState(false)
  const [selectedBackbone, setSelectedBackbone] = useState(null)
  const [cloudForm, setCloudForm] = useState({ name: '', host: '', subnets: '', listen_addr: '', username: 'root', password: '', auth_type: 'password', auto_deploy: true })
  const [leafForm, setLeafForm] = useState({ name: '', subnets: '' })

  async function fetchServers() {
    try {
      const res = await fetch('/api/servers')
      return res.json()
    } catch { return { cloud_servers: [], leaf_nodes: [] } }
  }

  const handleAddCloud = async (e) => {
    e.preventDefault()
    const res = await fetch('/api/servers/cloud', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cloudForm),
    })
    const data = await res.json()

    if (data.ssh_test || data.ssh_error) {
      setSshResult({
        ok: !data.ssh_error,
        test: data.ssh_test || '',
        error: data.ssh_error || '',
        steps: data.ssh_steps || '',
      })
    }

    if (data.node) {
      if (!cloudForm.auto_deploy || data.ssh_error) {
        setDeployScript(data.script)
        setShowDeploy(true)
      }
      setShowAddCloud(false)
      setCloudForm({ name: '', host: '', subnets: '', listen_addr: '', username: 'root', password: '', auth_type: 'password', auto_deploy: true })
    }
  }

  const handleAddLeaf = async (e) => {
    e.preventDefault()
    if (!selectedBackbone) return
    const res = await fetch('/api/servers/leaf', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...leafForm, backbone_id: selectedBackbone }),
    })
    const data = await res.json()
    if (data.node) {
      setDeployScript(data.script)
      setShowDeploy(true)
      setShowAddLeaf(false)
      setLeafForm({ name: '', subnets: '' })
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('确定删除此节点？关联隧道也会被删除。')) return
    await fetch(`/api/nodes/${id}`, { method: 'DELETE' })
  }

  const [deployModal, setDeployModal] = useState(null) // { id, host, creds }
  const [deployUser, setDeployUser] = useState('root')
  const [deployPass, setDeployPass] = useState('')
  const [deployErr, setDeployErr] = useState('')
  const [deploying, setDeploying] = useState(false)

  const handleDeploy = async (id, host) => {
    const creds = JSON.parse(localStorage.getItem('meshnet_ssh_' + host) || 'null')
    if (creds) {
      setDeployUser(creds.username)
      setDeployPass(creds.password)
      setDeployModal({ id, host, cached: true })
      tryAutoDeploy(id, host, creds.username, creds.password)
    } else {
      setDeployUser('root')
      setDeployPass('')
      setDeployErr('')
      setDeployModal({ id, host, cached: false })
    }
  }

  const tryAutoDeploy = async (id, host, username, password) => {
    setDeploying(true)
    setDeployErr('')
    const res = await fetch(`/api/servers/${id}/auto-deploy`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ host, username, password }),
    })
    const data = await res.json()
    setDeploying(false)
    if (data.deployed) {
      localStorage.setItem('meshnet_ssh_' + host, JSON.stringify({ username, password }))
      setDeployModal(null)
      alert(`部署成功! ${data.host}`)
    } else {
      setDeployErr(data.error || 'SSH 连接失败')
    }
  }

  const handleUpdate = async (id, ver) => {
    if (ver === latestVer) { alert('已是最新版本'); return }
    const res = await fetch('/api/agents/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ node_id: id }),
    })
    const data = await res.json()
    if (data.error) alert('更新失败: ' + data.error)
    else alert('更新指令已发送，Agent 将自动更新并重启')
  }

  const handleUpdateAll = async () => {
    if (!confirm('确定更新所有在线 Agent？')) return
    const res = await fetch('/api/agents/update-all', { method: 'POST' })
    const data = await res.json()
    alert(`已触发 ${data.updated} 个 Agent 更新`)
  }

  const handleServerUpdate = async (ver) => {
    if (ver === latestVer) { alert('控制端已是最新'); return }
    if (!confirm('确定更新控制端服务器？服务会短暂中断。')) return
    await fetch('/api/server/update', { method: 'POST' })
    alert('服务端更新中，稍后刷新页面...')
  }

  if (loading) return <div className="text-gray-500 text-sm p-6">加载中...</div>

  const cloudServers = data?.cloud_servers || []
  const leafNodes = data?.leaf_nodes || []

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xl font-bold">服务器管理</h2>
        <div className="flex gap-2">
          <button onClick={handleUpdateAll} className="px-3 py-1.5 text-xs bg-amber-600/20 text-amber-400 rounded hover:bg-amber-600/40 transition-colors">
            更新全部 Agent
          </button>
          <button onClick={() => handleServerUpdate(latestVer)} className="px-3 py-1.5 text-xs bg-purple-600/20 text-purple-400 rounded hover:bg-purple-600/40 transition-colors">
            更新控制端
          </button>
          <button onClick={() => setShowAddCloud(!showAddCloud)} className="px-3 py-1.5 text-xs bg-emerald-600 hover:bg-emerald-500 text-white rounded transition-colors">
            添加云服务器
          </button>
          <button onClick={() => {
            if (cloudServers.length === 0) { alert('请先添加云服务器'); return }
            setShowAddLeaf(!showAddLeaf)
            setSelectedBackbone(cloudServers[0].id)
          }} className="px-3 py-1.5 text-xs bg-blue-600 hover:bg-blue-500 text-white rounded transition-colors">
            添加叶子节点
          </button>
        </div>
      </div>

      {/* Add Cloud Server Form */}
      {showAddCloud && (
        <form onSubmit={handleAddCloud} className="bg-gray-900 border border-gray-800 rounded-lg p-4 mb-6 grid grid-cols-2 gap-3">
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="名称 (如: 北京云)" value={cloudForm.name} onChange={(e) => setCloudForm({ ...cloudForm, name: e.target.value })} required />
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="公网 IP" value={cloudForm.host} onChange={(e) => setCloudForm({ ...cloudForm, host: e.target.value })} required />
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="子网 CIDR (如 172.16.0.0/16)" value={cloudForm.subnets} onChange={(e) => setCloudForm({ ...cloudForm, subnets: e.target.value })} />
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="监听地址 (默认 :443)" value={cloudForm.listen_addr} onChange={(e) => setCloudForm({ ...cloudForm, listen_addr: e.target.value })} />

          <div className="col-span-2 border-t border-gray-800 pt-3 mt-1">
            <p className="text-[10px] text-gray-500 mb-2">SSH 凭据（用于全自动部署）</p>
          </div>
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="用户名 (默认 root)" value={cloudForm.username} onChange={(e) => setCloudForm({ ...cloudForm, username: e.target.value })} />
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="密码" type="password" value={cloudForm.password} onChange={(e) => setCloudForm({ ...cloudForm, password: e.target.value })} />
          <label className="flex items-center gap-2 text-xs text-gray-400 col-span-2">
            <input type="checkbox" checked={cloudForm.auto_deploy} onChange={(e) => setCloudForm({ ...cloudForm, auto_deploy: e.target.checked })} />
            添加后自动 SSH 部署 Agent
          </label>
          <div className="col-span-2 flex gap-2">
            <button type="submit" className="px-3 py-1.5 text-xs bg-emerald-600 hover:bg-emerald-500 text-white rounded">添加并部署</button>
            <button type="button" onClick={() => setShowAddCloud(false)} className="px-3 py-1.5 text-xs text-gray-400 hover:text-gray-200">取消</button>
          </div>
        </form>
      )}

      {/* Add Leaf Form */}
      {showAddLeaf && (
        <form onSubmit={handleAddLeaf} className="bg-gray-900 border border-gray-800 rounded-lg p-4 mb-6 grid grid-cols-2 gap-3">
          <div>
            <label className="text-[10px] text-gray-500 mb-1 block">所属骨干节点</label>
            <select className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200 w-full" value={selectedBackbone || ''} onChange={(e) => setSelectedBackbone(Number(e.target.value))}>
              {cloudServers.map((s) => (
                <option key={s.id} value={s.id}>{s.name} ({s.subnets || 'no subnet'})</option>
              ))}
            </select>
          </div>
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="名称 (如: 办公室PC)" value={leafForm.name} onChange={(e) => setLeafForm({ ...leafForm, name: e.target.value })} required />
          <input className="bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-xs text-gray-200" placeholder="子网 (留空自动继承骨干)" value={leafForm.subnets} onChange={(e) => setLeafForm({ ...leafForm, subnets: e.target.value })} />
          <div className="flex gap-2">
            <button type="submit" className="px-3 py-1.5 text-xs bg-blue-600 hover:bg-blue-500 text-white rounded">添加并部署</button>
            <button type="button" onClick={() => setShowAddLeaf(false)} className="px-3 py-1.5 text-xs text-gray-400 hover:text-gray-200">取消</button>
          </div>
        </form>
      )}

      {/* Cloud Servers */}
      <h3 className="text-sm font-bold text-gray-300 mb-3 flex items-center gap-2">
        <span className="w-2 h-2 rounded-full bg-emerald-400" />
        云服务器 (骨干节点)
      </h3>
      <div className="grid grid-cols-1 gap-2 mb-8">
        {cloudServers.map((s) => (
          <div key={s.id} className="bg-gray-900 border border-gray-800 rounded-lg p-4">
            <div className="flex items-center justify-between">
              <div>
                <div className="flex items-center gap-3 mb-1">
                  <span className="text-sm font-medium text-gray-200">{s.name}</span>
                  <StatusBadge online={s.online} />
                  <span className="text-[10px] text-gray-500 bg-gray-800 px-1.5 py-0.5 rounded">骨干</span>
                </div>
                <div className="text-[10px] text-gray-500 space-x-3">
                  <span>{s.host || ''}</span>
                  <span>虚拟IP: {s.virtual_ip || '-'}</span>
                  <span>子网: {s.subnets || '-'}</span>
                  <span>接入: {s.tunnel_count || 0} 个叶子</span>
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={() => handleDeploy(s.id, s.host)} className="px-2 py-1 text-[10px] bg-emerald-600/20 text-emerald-400 rounded hover:bg-emerald-600/40">部署</button>
                {s.online && s.version && s.version !== latestVer && (
                  <button onClick={() => handleUpdate(s.id, s.version)} className="px-2 py-1 text-[10px] bg-amber-600/20 text-amber-400 rounded hover:bg-amber-600/40">更新</button>
                )}
                <button onClick={() => handleDelete(s.id)} className="px-2 py-1 text-[10px] bg-red-600/20 text-red-400 rounded hover:bg-red-600/40">删除</button>
              </div>
            </div>
              <div className="mt-2 text-[10px] text-gray-500">
                版本: {s.version || '-'}
                {s.version && s.version !== latestVer && s.online && <span className="text-amber-400 ml-1">(可更新)</span>}
              </div>
          </div>
        ))}
        {cloudServers.length === 0 && <p className="text-xs text-gray-500 py-4 text-center">暂无云服务器</p>}
      </div>

      {/* Leaf Nodes */}
      <h3 className="text-sm font-bold text-gray-300 mb-3 flex items-center gap-2">
        <span className="w-2 h-2 rounded-full bg-blue-400" />
        叶子节点 (终端主机)
      </h3>
      <div className="grid grid-cols-1 gap-2">
        {leafNodes.map((n) => (
          <div key={n.id} className="bg-gray-900 border border-gray-800 rounded-lg p-3">
            <div className="flex items-center justify-between">
              <div>
                <div className="flex items-center gap-3">
                  <span className="text-xs font-medium text-gray-200">{n.name}</span>
                  <StatusBadge online={n.online} />
                  <span className="text-[10px] text-gray-500 bg-gray-800 px-1.5 py-0.5 rounded">叶子</span>
                </div>
                <div className="text-[10px] text-gray-500 mt-0.5">
                  虚拟IP: {n.virtual_ip || '-'} · 子网: {n.subnets || '-'}
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={() => handleDeploy(n.id)} className="px-2 py-1 text-[10px] bg-blue-600/20 text-blue-400 rounded hover:bg-blue-600/40">部署</button>
                <button onClick={() => handleDelete(n.id)} className="px-2 py-1 text-[10px] bg-red-600/20 text-red-400 rounded hover:bg-red-600/40">删除</button>
              </div>
            </div>
          </div>
        ))}
        {leafNodes.length === 0 && <p className="text-xs text-gray-500 py-4 text-center">暂无叶子节点</p>}
      </div>

      <DeployModal open={showDeploy} onClose={() => setShowDeploy(false)} script={deployScript} />

      {/* SSH Credentials Modal */}
      {deployModal && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setDeployModal(null)}>
          <form onSubmit={(e) => { e.preventDefault(); tryAutoDeploy(deployModal.id, deployModal.host, deployUser, deployPass) }}
            className="bg-gray-900 border border-gray-700 rounded-xl w-80 p-4" onClick={e => e.stopPropagation()}>
            <h3 className="text-sm font-bold text-gray-200 mb-3">SSH 部署 {deployModal.host}</h3>
            <input className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-xs text-gray-200 mb-2"
              placeholder="用户名" value={deployUser} onChange={e => setDeployUser(e.target.value)} />
            <input className="w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-xs text-gray-200 mb-3"
              type="password" placeholder="密码" value={deployPass} onChange={e => setDeployPass(e.target.value)} />
            {deployErr && <p className="text-[11px] text-red-400 mb-2">{deployErr}</p>}
            <div className="flex gap-2">
              <button type="submit" disabled={deploying}
                className="flex-1 px-3 py-2 text-xs bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white rounded">
                {deploying ? '连接中...' : '部署'}
              </button>
              <button type="button" onClick={() => setDeployModal(null)}
                className="px-3 py-2 text-xs text-gray-400 hover:text-gray-200">取消</button>
            </div>
          </form>
        </div>
      )}

      {/* SSH Result */}
      {sshResult && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setSshResult(null)}>
          <div className={`bg-gray-900 border rounded-lg w-full max-w-md mx-4 p-4 ${sshResult.ok ? 'border-emerald-700' : 'border-red-700'}`} onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-between mb-3">
              <h3 className={`text-sm font-bold ${sshResult.ok ? 'text-emerald-400' : 'text-red-400'}`}>
                {sshResult.ok ? '自动部署完成' : '部署失败'}
              </h3>
              <button onClick={() => setSshResult(null)} className="text-gray-500 hover:text-gray-300">&times;</button>
            </div>
            {sshResult.test && <p className="text-[10px] text-gray-500 mb-2">{sshResult.test}</p>}
            {sshResult.steps && <p className="text-xs text-gray-300 mb-2">{sshResult.steps}</p>}
            {sshResult.error && <p className="text-xs text-red-400">{sshResult.error}</p>}
            {sshResult.ok && <p className="text-[10px] text-emerald-400 mt-2">Agent 已安装并启动，等待连接...</p>}
          </div>
        </div>
      )}
    </div>
  )
}
