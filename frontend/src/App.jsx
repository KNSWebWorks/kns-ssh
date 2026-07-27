import { useState, useEffect, useRef, useCallback } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

const AUTH_KEY = 'kns_auth'
const sessionsKey = (agentToken) => `kns_sessions_${agentToken}`

function loadAuth() {
  try {
    return JSON.parse(localStorage.getItem(AUTH_KEY))
  } catch {
    return null
  }
}

function loadSessions(agentToken) {
  try {
    return JSON.parse(localStorage.getItem(sessionsKey(agentToken))) || []
  } catch {
    return []
  }
}

function newSessionId() {
  return `t-${Date.now().toString(36)}-${Math.random().toString(36).substring(2, 8)}`
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
          <AgentConsolesView agent={selectedAgent} onBack={() => setSelectedAgent(null)} />
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
      if (!res.ok) throw new Error('Invalid email or password')
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

// AgentConsolesView manages multiple terminal tabs for one agent.
// Tabs survive page reloads: session ids are stored in localStorage and
// reattached (the agent replays the scrollback).
function AgentConsolesView({ agent, onBack }) {
  const [tabs, setTabs] = useState(() => loadSessions(agent.token))
  const [activeId, setActiveId] = useState(() => loadSessions(agent.token)[0]?.id || null)
  const tabApis = useRef({}) // sessionId -> { restart, kill }

  const persist = (next) => {
    setTabs(next)
    localStorage.setItem(sessionsKey(agent.token), JSON.stringify(next))
  }

  const addTab = () => {
    const id = newSessionId()
    const next = [...tabs, { id, title: `term ${tabs.length + 1}` }]
    persist(next)
    setActiveId(id)
  }

  const closeTab = (id) => {
    tabApis.current[id]?.kill() // explicitly closing a tab kills its shell
    delete tabApis.current[id]
    const next = tabs.filter(t => t.id !== id)
    persist(next)
    if (activeId === id) setActiveId(next[next.length - 1]?.id || null)
  }

  const registerApi = (id, api) => { tabApis.current[id] = api }
  const unregisterApi = (id) => { delete tabApis.current[id] }

  return (
    <>
      <div className="glass-panel" style={{ display: 'flex', gap: '10px', alignItems: 'center', marginBottom: '14px', flexWrap: 'wrap' }}>
        <button className="btn btn-secondary" onClick={onBack}>← Back</button>
        <strong>{agent.name}</strong>
        <div style={{ flex: 1 }}></div>
        <button className="btn btn-secondary" onClick={() => tabApis.current[activeId]?.restart()} disabled={!activeId}>
          ⟳ Restart
        </button>
        <button className="btn" onClick={addTab}>+ New terminal</button>
      </div>

      {tabs.length > 0 && (
        <div className="tab-bar">
          {tabs.map(t => (
            <div
              key={t.id}
              className={`tab ${t.id === activeId ? 'active' : ''}`}
              onClick={() => setActiveId(t.id)}
            >
              <span>{t.title}</span>
              <span
                className="tab-close"
                title="Close terminal"
                onClick={(e) => { e.stopPropagation(); closeTab(t.id) }}
              >×</span>
            </div>
          ))}
        </div>
      )}

      {tabs.length === 0 && (
        <div className="glass-panel" style={{ textAlign: 'center', color: 'var(--text-muted)', padding: '60px 20px' }}>
          No open terminals. Click <strong>+ New terminal</strong> to start one.
        </div>
      )}

      {/* All tabs stay mounted to preserve xterm state; inactive ones are hidden.
          Unmounting (Back / reload) does NOT kill shells — they live on the agent
          and are reattached when you come back. */}
      {tabs.map(t => (
        <div key={t.id} style={{ display: t.id === activeId ? 'flex' : 'none', flex: 1, flexDirection: 'column' }}>
          <TerminalTab
            agent={agent}
            sessionId={t.id}
            isActive={t.id === activeId}
            registerApi={registerApi}
            unregisterApi={unregisterApi}
          />
        </div>
      ))}
    </>
  )
}

function TerminalTab({ agent, sessionId, isActive, registerApi, unregisterApi }) {
  const [connected, setConnected] = useState(false)
  const terminalRef = useRef(null)
  const termRef = useRef(null)
  const wsRef = useRef(null)
  const fitRef = useRef(null)
  const closedByUser = useRef(false)
  const reconnectTimer = useRef(null)

  useEffect(() => {
    const term = new Terminal({
      theme: { background: '#0f172a', foreground: '#f8fafc', cursor: '#3b82f6' },
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      scrollback: 5000,
    })
    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(terminalRef.current)
    fitAddon.fit()
    termRef.current = term
    fitRef.current = fitAddon

    const send = (obj) => {
      const ws = wsRef.current
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(obj))
      }
    }

    // Ctrl/Cmd key handling: browsers steal Ctrl+C/V/Z etc.
    // With a selection Ctrl+C copies; otherwise it sends SIGINT like a real terminal.
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== 'keydown') return true
      const key = e.key.toLowerCase()
      const ctrl = e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey
      const cmd = e.metaKey && !e.shiftKey && !e.altKey && !e.ctrlKey
      if (ctrl || cmd) {
        const codes = { c: '\x03', z: '\x1a', x: '\x18', a: '\x01', d: '\x04', l: '\x0c', e: '\x05', u: '\x15', k: '\x0b', w: '\x17', r: '\x12' }
        if (key === 'c' && term.hasSelection()) {
          navigator.clipboard.writeText(term.getSelection())
          return false
        }
        if (key === 'v') {
          navigator.clipboard.readText().then(t => t && send({ type: 'terminal_data', data: t }))
          return false
        }
        if (codes[key]) {
          send({ type: 'terminal_data', data: codes[key] })
          return false
        }
      }
      return true
    })

    term.onData(data => send({ type: 'terminal_data', data }))

    const handleResize = () => {
      fitAddon.fit()
      send({ type: 'resize', cols: term.cols, rows: term.rows })
    }
    window.addEventListener('resize', handleResize)

    const connect = () => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/client?session_id=${sessionId}&agent_id=${agent.token}`)
      wsRef.current = ws

      ws.onopen = () => {
        setConnected(true)
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          switch (msg.type) {
            case 'terminal_data':
              term.write(msg.data)
              break
            case 'replay':
              term.reset()
              if (msg.data) term.write(msg.data)
              break
            case 'session_started':
              term.reset()
              break
            case 'session_closed':
              term.write('\r\n\x1b[31m[Shell exited — press ⟳ Restart to start a new one]\x1b[0m\r\n')
              break
          }
        } catch (e) {
          console.error('Failed to parse msg:', e)
        }
      }

      ws.onclose = () => {
        setConnected(false)
        if (!closedByUser.current) {
          // Auto-reconnect (reattaches to the same session on the agent)
          reconnectTimer.current = setTimeout(connect, 3000)
        }
      }
    }
    connect()

    // API for the parent: restart kills the shell and starts a fresh one;
    // kill is used only when the user explicitly closes the tab.
    registerApi(sessionId, {
      restart: () => {
        term.reset()
        send({ type: 'restart_session' })
      },
      kill: () => {
        closedByUser.current = true
        clearTimeout(reconnectTimer.current)
        send({ type: 'kill_session' })
        wsRef.current?.close()
      },
    })

    return () => {
      // Unmount (Back navigation / page teardown) leaves the shell ALIVE
      // on the agent so the session can be reattached later.
      closedByUser.current = true
      clearTimeout(reconnectTimer.current)
      window.removeEventListener('resize', handleResize)
      unregisterApi(sessionId)
      wsRef.current?.close()
      term.dispose()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent.token, sessionId])

  // Refit when the tab becomes visible again
  useEffect(() => {
    if (isActive && fitRef.current) {
      fitRef.current.fit()
      termRef.current?.focus()
    }
  }, [isActive])

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '8px' }}>
        <span className={`status-dot ${connected ? 'online' : 'offline'}`}></span>
        <span style={{ color: 'var(--text-muted)', fontSize: '13px' }}>
          {connected ? 'Connected' : 'Reconnecting...'}
        </span>
      </div>
      <div className="glass-panel" style={{ flex: 1, padding: '10px', minHeight: '420px', display: 'flex' }}>
        <div ref={terminalRef} style={{ width: '100%', height: '100%' }}></div>
      </div>
    </>
  )
}

export default App
