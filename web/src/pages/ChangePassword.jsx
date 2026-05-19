import { useState } from 'react'
import { useAuth } from '../context/AuthContext'
import { useNavigate } from 'react-router-dom'

export default function ChangePassword() {
  const { changePassword, user } = useAuth()
  const navigate = useNavigate()
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')

    if (newPassword !== confirmPassword) {
      setError('两次输入的密码不一致')
      return
    }
    if (newPassword.length < 6) {
      setError('密码至少 6 位')
      return
    }

    setBusy(true)
    try {
      await changePassword(oldPassword, newPassword)
      navigate('/')
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
          <h1 className="text-xl font-bold text-emerald-400">首次登录</h1>
          <p className="text-gray-500 text-sm mt-1">
            你好 {user?.username}，请修改默认密码
          </p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm text-gray-400 mb-1">当前密码</label>
            <input
              type="password"
              value={oldPassword}
              onChange={(e) => setOldPassword(e.target.value)}
              className="w-full px-3 py-2 bg-gray-900 border border-gray-800 rounded text-gray-200 focus:outline-none focus:border-emerald-500"
              autoFocus
            />
          </div>

          <div>
            <label className="block text-sm text-gray-400 mb-1">新密码</label>
            <input
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className="w-full px-3 py-2 bg-gray-900 border border-gray-800 rounded text-gray-200 focus:outline-none focus:border-emerald-500"
              placeholder="至少 6 位"
            />
          </div>

          <div>
            <label className="block text-sm text-gray-400 mb-1">确认新密码</label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className="w-full px-3 py-2 bg-gray-900 border border-gray-800 rounded text-gray-200 focus:outline-none focus:border-emerald-500"
              placeholder="再次输入新密码"
            />
          </div>

          {error && <p className="text-red-400 text-sm">{error}</p>}

          <button
            type="submit"
            disabled={busy}
            className="w-full py-2 bg-emerald-600 hover:bg-emerald-500 disabled:bg-gray-700 text-white rounded font-medium transition-colors"
          >
            {busy ? '修改中...' : '确认修改'}
          </button>
        </form>
      </div>
    </div>
  )
}
