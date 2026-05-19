import { useState } from 'react'
import { useAuth } from '../context/AuthContext'
import { useNavigate } from 'react-router-dom'

export default function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      const data = await login(username, password)
      if (data.must_change_pass) {
        navigate('/change-password')
      } else {
        navigate('/')
      }
    } catch (err) {
      setError(err.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="min-h-screen bg-gray-950 flex items-center justify-center">
      <div className="w-full max-w-sm p-8">
        <div className="text-center mb-8">
          <h1 className="text-2xl font-bold text-emerald-400">MeshNet</h1>
          <p className="text-gray-500 text-sm mt-1">隐蔽 Mesh 网络管理系统</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm text-gray-400 mb-1">用户名</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full px-3 py-2 bg-gray-900 border border-gray-800 rounded text-gray-200 focus:outline-none focus:border-emerald-500"
              placeholder="admin"
              autoFocus
            />
          </div>

          <div>
            <label className="block text-sm text-gray-400 mb-1">密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full px-3 py-2 bg-gray-900 border border-gray-800 rounded text-gray-200 focus:outline-none focus:border-emerald-500"
              placeholder="••••••"
            />
          </div>

          {error && <p className="text-red-400 text-sm">{error}</p>}

          <button
            type="submit"
            disabled={busy}
            className="w-full py-2 bg-emerald-600 hover:bg-emerald-500 disabled:bg-gray-700 text-white rounded font-medium transition-colors"
          >
            {busy ? '登录中...' : '登录'}
          </button>
        </form>
      </div>
    </div>
  )
}
