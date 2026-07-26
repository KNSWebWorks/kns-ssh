import { useState, useEffect } from 'react'

function App() {
  const [servers, setServers] = useState([]);
  const [tools, setTools] = useState([]);
  const [selectedServer, setSelectedServer] = useState('');
  const [selectedTool, setSelectedTool] = useState('');
  const [output, setOutput] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    // Fetch servers and tools
    const fetchData = async () => {
      try {
        const [serversRes, toolsRes] = await Promise.all([
          fetch('/api/collections/servers/records'),
          fetch('/api/collections/tools/records')
        ]);
        
        if (serversRes.ok) {
          const serversData = await serversRes.json();
          setServers(serversData.items || []);
        }
        if (toolsRes.ok) {
          const toolsData = await toolsRes.json();
          setTools(toolsData.items || []);
        }
      } catch (err) {
        console.error("Failed to fetch data:", err);
        setError("Could not load data. Ensure you are logged in to PocketBase admin and collections are created.");
      }
    };
    fetchData();
  }, []);

  const handleExecute = async () => {
    if (!selectedServer || !selectedTool) {
      setError("Please select both a server and a tool.");
      return;
    }

    setLoading(true);
    setError(null);
    setOutput('Executing...');

    try {
      const res = await fetch('/api/ssh/execute', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          // Assuming user is authenticated via PocketBase cookie or we are ignoring auth for demo
        },
        body: JSON.stringify({
          server_id: selectedServer,
          tool_id: selectedTool,
        })
      });

      const data = await res.json();
      
      if (!res.ok) {
        setError(data.error || "Execution failed");
        setOutput(data.details ? data.details.Stderr || data.details.Error : "");
      } else {
        setOutput(data.Stdout + (data.Stderr ? `\n\n[STDERR]\n${data.Stderr}` : ""));
      }
    } catch (err) {
      setError("Network error: " + err.message);
      setOutput("");
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <header className="header">
        <h2>SSH Assistant</h2>
        <div style={{color: "var(--text-muted)", fontSize: "14px"}}>Orchestration Hub</div>
      </header>

      <main className="main-content">
        <aside className="sidebar-nav glass-panel">
          <h3>Dashboard</h3>
          <div style={{marginTop: "20px"}}>
            <div className="nav-item active">CLI Executor</div>
            <div className="nav-item">Servers List</div>
            <div className="nav-item">Tools Config</div>
          </div>
        </aside>

        <section className="glass-panel" style={{display: "flex", flexDirection: "column", gap: "20px"}}>
          <div>
            <h1 style={{marginBottom: "8px"}}>Remote Execution</h1>
            <p style={{color: "var(--text-muted)", fontSize: "14px"}}>Select a server and a CLI tool to execute the command.</p>
          </div>

          {error && (
            <div style={{padding: "12px", background: "rgba(239, 68, 68, 0.2)", border: "1px solid var(--error)", borderRadius: "8px", color: "var(--error)"}}>
              {error}
            </div>
          )}

          <div style={{display: "flex", gap: "16px"}}>
            <div className="form-group" style={{flex: 1}}>
              <label>Select Server</label>
              <select value={selectedServer} onChange={(e) => setSelectedServer(e.target.value)}>
                <option value="">-- Select Server --</option>
                {servers.map(s => (
                  <option key={s.id} value={s.id}>{s.name} ({s.host})</option>
                ))}
              </select>
            </div>

            <div className="form-group" style={{flex: 1}}>
              <label>Select Tool</label>
              <select value={selectedTool} onChange={(e) => setSelectedTool(e.target.value)}>
                <option value="">-- Select Tool --</option>
                {tools.map(t => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            </div>
          </div>

          <div>
            <button className="btn" onClick={handleExecute} disabled={loading}>
              {loading ? "Running..." : "Execute Command"}
            </button>
          </div>

          <div style={{marginTop: "10px"}}>
            <label style={{display: "block", marginBottom: "8px", fontSize: "14px", color: "var(--text-muted)"}}>Output Console</label>
            <div className="terminal-output">
              {output || "Waiting for input..."}
            </div>
          </div>
        </section>
      </main>
    </>
  )
}

export default App
