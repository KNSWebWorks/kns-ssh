import { useState, useEffect, useRef, useCallback } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

const AUTH_KEY = 'kns_auth'

function loadAuth() {
  try {
    return JSON.parse(localStorage.getItem(AUTH_KEY))
  } catch {
    return null
  }
}

function App() {
  const [auth, setAuth] = useState(loadAuth)
  const [selectedAgent, setSelectedAgent] = useState(null)

  const handleLogout = () => {
    localStorage.removeItem(AUTH_KEY)
    setAuth(null)
    setSelectedAgent(null)
  }

  return (
    <>
      <header className="header">
        <h2>KNS SSH</h2>
        {auth && (
          <div style={{ display: 'flex', alignItems: 'center', gap: '14px' }}>
            <span style={{ color: 'var(--text-muted)', fontSize: '14px' }}>{auth.record?.email}</span>
            <button className="btn btn-secondary" onClick={handleLogout}>Log out</button>
          </div>
        )}
      </header>

      <main className="main-content" style={{ display: 'flex', flexDirection: 'column' }}>
        {!auth ? (
          <LoginView onLogin={setAuth} />
        ) : selectedAgent ? (
          <TerminalView agent={selectedAgent} onBack={() => setSelectedAgent(null)} />
        ) : (
          <AgentListView auth={auth} onSelect={setSelectedAgent} onAuthExpired={handleLogout} />
        )}
      </main>
    </>
  )
}

function LoginView({ onLogin }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await fetch('/api/collections/users/auth-with-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ identity: email, password }),
      })
      if (!res.ok) {
        throw new Error('Invalid email or password')
      }
      const data = await res.json()
      localStorage.setItem(AUTH_KEY, JSON.stringify(data))
      onLogin(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ maxWidth: '400px', margin: '60px auto', width: '100%' }}>
      <form className="glass-panel" onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
        <h3 style={{ textAlign: 'center' }}>Sign in</h3>
        <input type="email" placeholder="Email" value={email} onChange={e => setEmail(e.target.value)} required autoFocus />
        <input type="password" placeholder="Password" value={password} onChange={e => setPassword(e.target.value)} required />
        <button className="btn" type="submit" disabled={loading}>
          {loading ? 'Signing in...' : 'Sign in'}
        </button>
        {error && <div className="form-error">{error}</div>}
      </form>
    </div>
  )
}

function AgentListView({ auth, onSelect, onAuthExpired }) {
  const [agents, setAgents] = useState(null)
  const [error, setError] = useState('')

  const fetchAgents = useCallback(async () => {
    try {
      const res = await fetch('/api/agents/online', {
        headers: { Authorization: auth.token },
      })
      if (res.status === 401 || res.status === 403) {
        onAuthExpired()
        return
      }
      if (!res.ok) throw new Error('Failed to load agents')
      setAgents(await res.json())
    } catch (err) {
      setError(err.message)
    }
  }, [auth, onAuthExpired])

  useEffect(() => {
    fetchAgents()
    const interval = setInterval(fetchAgents, 5000)
    return () => clearInterval(interval)
  }, [fetchAgents])

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3>Your computers</h3>
        <button className="btn btn-secondary" onClick={fetchAgents}>Refresh</button>
      </div>

      {error && <div className="form-error">{error}</div>}
      {agents && agents.length === 0 && (
        <p style={{ color: 'var(--text-muted)', marginTop: '20px' }}>
          No agents registered yet. Ask your admin to run <code>kns-ssh create-agent</code>.
        </p>
      )}

      <div className="agent-grid">
        {agents?.map(agent => (
          <div
            key={agent.token}
            className={`agent-card ${agent.online ? '' : 'offline'}`}
            onClick={() => agent.online && onSelect(agent)}
          >
            <div className="agent-name">
              <span className={`status-dot ${agent.online ? 'online' : 'offline'}`}></span>
              {agent.name}
            </div>
            <div className="agent-status">{agent.online ? 'Online — click to connect' : 'Offline'}</div>
          </div>
        ))}
      </div>
    </div>
  )
}

function TerminalView({ agent, onBack }) {
  const [connected, setConnected] = useState(false)
  const terminalRef = useRef(null)
  const termInstance = useRef(null)

  useEffect(() => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const sessionId = Math.random().toString(36).substring(2)
    const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/client?session_id=${sessionId}&agent_id=${agent.token}`)

    const term = new Terminal({
      theme: { background: '#0f172a', foreground: '#f8fafc', cursor: '#3b82f6' },
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    })
    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(terminalRef.current)
    fitAddon.fit()
    termInstance.current = term

    const handleResize = () => {
      fitAddon.fit()
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }
    window.addEventListener('resize', handleResize)

    term.onData(data => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'terminal_data', data }))
      }
    })

    ws.onopen = () => {
      setConnected(true)
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
    }

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        if (msg.type === 'terminal_data') {
          term.write(msg.data)
        }
      } catch (e) {
        console.error('Failed to parse msg:', e)
      }
    }

    ws.onclose = () => {
      setConnected(false)
      term.write('\r\n\x1b[31m[Disconnected from agent]\x1b[0m\r\n')
    }

    return () => {
      window.removeEventListener('resize', handleResize)
      ws.close()
      term.dispose()
    }
  }, [agent])

  return (
    <>
      <div className="glass-panel" style={{ display: 'flex', gap: '10px', alignItems: 'center', marginBottom: '20px' }}>
        <button className="btn btn-secondary" onClick={onBack}>← Back</button>
        <span className={`status-dot ${connected ? 'online' : 'offline'}`}></span>
        <strong>{agent.name}</strong>
        <span style={{ color: 'var(--text-muted)', fontSize: '13px' }}>
          {connected ? 'Connected' : 'Connecting...'}
        </span>
      </div>

      <div className="glass-panel" style={{ flex: 1, padding: '10px', minHeight: '500px', display: 'flex' }}>
        <div ref={terminalRef} style={{ width: '100%', height: '100%' }}></div>
      </div>
    </>
  )
}

export default App
