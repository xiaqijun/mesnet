import { useState, useEffect } from 'react'

export function usePolling(fn, interval = 3000) {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let active = true

    const poll = async () => {
      try {
        const result = await fn()
        if (active) setData(result)
      } catch (e) {
        console.error('poll error:', e)
      } finally {
        if (active) setLoading(false)
      }
    }

    poll()
    const timer = setInterval(poll, interval)
    return () => {
      active = false
      clearInterval(timer)
    }
  }, [interval])

  return { data, loading }
}
