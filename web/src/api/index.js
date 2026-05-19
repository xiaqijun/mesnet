const BASE = '/api'

function getToken() {
  return localStorage.getItem('mesnet_token')
}

async function req(url, opts = {}) {
  const headers = { 'Content-Type': 'application/json', ...opts.headers }
  const token = getToken()
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  const res = await fetch(BASE + url, { headers, ...opts })
  if (res.status === 401) {
    // Token expired or invalid — clear auth and redirect
    localStorage.removeItem('mesnet_token')
    localStorage.removeItem('mesnet_user')
    window.location.href = '/login'
    throw new Error('unauthorized')
  }
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json()
}

export const api = {
  // Dashboard
  getStats: () => req('/stats'),

  // Nodes
  listNodes: () => req('/nodes'),
  getNode: (id) => req(`/nodes/${id}`),
  createNode: (data) => req('/nodes', { method: 'POST', body: JSON.stringify(data) }),
  updateNode: (id, data) => req(`/nodes/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteNode: (id) => req(`/nodes/${id}`, { method: 'DELETE' }),
  getDeployScript: (id) => req(`/nodes/${id}/deploy`),
  getNodeStats: (id) => req(`/nodes/${id}/stats`),

  // Tunnels
  listTunnels: () => req('/tunnels'),
  getTunnel: (id) => req(`/tunnels/${id}`),
  createTunnel: (data) => req('/tunnels', { method: 'POST', body: JSON.stringify(data) }),
  deleteTunnel: (id) => req(`/tunnels/${id}`, { method: 'DELETE' }),
  tunnelUp: (id) => req(`/tunnels/${id}/up`, { method: 'POST' }),
  tunnelDown: (id) => req(`/tunnels/${id}/down`, { method: 'POST' }),
  getTunnelStats: (id) => req(`/tunnels/${id}/stats`),
  getTunnelHistory: (id, range) => req(`/tunnels/${id}/stats/history?range=${range || '1h'}`),

  // Topology
  getTopology: () => req('/topology'),

  // Monitor
  getTotalTraffic: () => req('/monitor/total'),

  // Audit
  listAudit: () => req('/audit'),
}
