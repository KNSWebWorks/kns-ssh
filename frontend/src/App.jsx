import { useState, useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

function App() {
  const [agentId, setAgentId] = useState('')
  const [connected, setConnected] = useState(false)
  const terminalRef = useRef(null)
  const termInstance = useRef(null)
  const wsRef = useRef(null)

  const handleConnect = () => {
    if (!agentId) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/api/ws/client?session_id=${Math.random().toString(36).substring(7)}&agent_id=${agentId}`;
    
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      
      // Init xterm
      if (!termInstance.current) {
        const term = new Terminal({
          theme: {
            background: '#0f172a',
            foreground: '#f8fafc',
            cursor: '#3b82f6'
          },
          fontFamily: 'Menlo, Monaco, "Courier New", monospace'
        });
        const fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        
        term.open(terminalRef.current);
        fitAddon.fit();
        termInstance.current = term;

        // Handle typing
        term.onData(data => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'terminal_data', data }));
          }
        });

        // Handle resize
        window.addEventListener('resize', () => {
          fitAddon.fit();
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
              type: 'resize',
              cols: term.cols,
              rows: term.rows
            }));
          }
        });
      }
    };

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'terminal_data' && termInstance.current) {
          termInstance.current.write(msg.data);
        }
      } catch (e) {
        console.error("Failed to parse msg:", e);
      }
    };

    ws.onclose = () => {
      setConnected(false);
      if (termInstance.current) {
        termInstance.current.write('\r\n\x1b[31m[Disconnected from agent]\x1b[0m\r\n');
      }
    };
  };

  return (
    <>
      <header className="header">
        <h2>KNS SSH</h2>
        <div style={{color: "var(--text-muted)", fontSize: "14px"}}>Agent Control Center</div>
      </header>

      <main className="main-content" style={{display: 'flex', flexDirection: 'column'}}>
        <div className="glass-panel" style={{display: "flex", gap: "10px", alignItems: "center", marginBottom: "20px"}}>
          <input 
            type="text" 
            placeholder="Enter Agent Token..." 
            value={agentId} 
            onChange={e => setAgentId(e.target.value)}
            disabled={connected}
            style={{maxWidth: "300px"}}
          />
          <button className="btn" onClick={handleConnect} disabled={connected || !agentId}>
            {connected ? "Connected" : "Connect to Agent"}
          </button>
        </div>

        <div 
          className="glass-panel" 
          style={{ flex: 1, padding: '10px', minHeight: '500px', display: 'flex' }}
        >
          <div ref={terminalRef} style={{ width: '100%', height: '100%' }}></div>
        </div>
      </main>
    </>
  )
}

export default App
