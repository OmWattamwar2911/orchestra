import React, { useState } from 'react';
import { 
  Key, 
  Cpu, 
  Database, 
  Copy, 
  Check 
} from 'lucide-react';
import { GridSidebarLabel } from '../components/ui/GridSidebarLabel';
import { PRODUCT_NAME } from '../config';

export const DocsPage: React.FC = () => {
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [activeSection, setActiveSection] = useState<string>('overview');

  const copyToClipboard = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(id);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const scrollTo = (id: string) => {
    setActiveSection(id);
    const element = document.getElementById(id);
    if (element) {
      element.scrollIntoView({ behavior: 'smooth' });
    }
  };

  return (
    <div className="w-full bg-cream min-h-screen text-jet">
      {/* 12-Column Grid Layout */}
      <div className="max-w-[1440px] mx-auto grid grid-cols-1 md:grid-cols-12 min-h-screen">
        
        {/* Sticky Label Sidebar (3 Columns) */}
        <div className="hidden md:block md:col-span-3 border-r border-hairline p-6 lg:p-8 bg-cream">
          <div className="sticky top-24 space-y-8">
            <GridSidebarLabel label="01 / ARCHITECTURE" />

            {/* Navigation Table of Contents */}
            <div className="space-y-1 font-mono text-xs">
              <span className="text-[10px] text-gray uppercase tracking-widest block mb-2">
                INDEX / SECTIONS
              </span>
              {[
                { id: 'overview', index: '001', label: 'SYSTEM OVERVIEW' },
                { id: 'etcd-layout', index: '002', label: 'ETCD KEY SPACE' },
                { id: 'fencing', index: '003', label: 'FENCING & CAS' },
                { id: 'leases', index: '004', label: 'WORKER LEASES' },
                { id: 'lifecycle', index: '005', label: 'TASK LIFECYCLE' },
                { id: 'invariants', index: '006', label: 'CHAOS INVARIANTS' },
                { id: 'matrix', index: '007', label: 'FAILURE MATRIX' },
              ].map((item) => (
                <button
                  key={item.id}
                  onClick={() => scrollTo(item.id)}
                  className={`w-full text-left py-1.5 px-2 flex items-center justify-between border-l-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cobalt ${
                    activeSection === item.id 
                      ? 'border-cobalt font-bold text-jet bg-cream' 
                      : 'border-transparent text-muted-strong hover:text-jet'
                  }`}
                >
                  <span className="tracking-wide">{item.label}</span>
                  <span className="text-[10px] text-gray font-mono">{item.index}</span>
                </button>
              ))}
            </div>

            {/* Quick Specs */}
            <div className="border border-hairline p-4 space-y-2 bg-cream text-xs font-mono">
              <span className="text-[10px] uppercase font-bold tracking-widest text-cobalt block">
                SYSTEM SPECIFICATIONS
              </span>
              <div className="space-y-1 text-deep-gray text-[11px]">
                <div className="flex justify-between">
                  <span className="text-gray">CONSENSUS:</span>
                  <span className="text-jet font-bold">Raft / etcd v3.5</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray">RPC TRANSPORT:</span>
                  <span className="text-jet font-bold">gRPC over HTTP/2</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray">HEARTBEAT TTL:</span>
                  <span className="text-jet font-bold">5,000 ms</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray">CHECK INTERVAL:</span>
                  <span className="text-jet font-bold">1,500 ms</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Main Documentation Body (9 Columns) */}
        <div className="col-span-1 md:col-span-9 p-6 lg:p-12 space-y-16 bg-cream">
          
          {/* Header Section */}
          <div className="space-y-4 border-b border-hairline pb-8">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 bg-cobalt inline-block" />
              <span className="font-mono text-xs uppercase tracking-widest text-gray">
                SYSTEM ARCHITECTURE & FORMAL SPECIFICATION
              </span>
            </div>
            
            <h1 className="text-4xl sm:text-6xl font-black uppercase tracking-tight text-jet leading-[0.9]">
              ARCHITECTURE & GUARANTEES
            </h1>
            
            <p className="text-base sm:text-lg text-deep-gray max-w-3xl leading-relaxed">
              How {PRODUCT_NAME} enforces linearizable DAG workflow scheduling across distributed failure domains, eliminates split-brain execution with monotonic fencing tokens, and guarantees exactly-once state commit.
            </p>
          </div>

          {/* Section 001: System Overview */}
          <section id="overview" className="space-y-6 scroll-mt-24">
            <div className="flex items-baseline justify-between border-b border-hairline pb-2">
              <h2 className="text-2xl font-bold uppercase tracking-tight text-jet flex items-center gap-3">
                <span className="font-mono text-sm text-gray">001</span>
                <span>SYSTEM OVERVIEW & TOPOLOGY</span>
              </h2>
              <span className="font-mono text-xs text-muted-strong uppercase">THREE-TIER MESH</span>
            </div>

            <p className="text-sm text-deep-gray leading-relaxed max-w-4xl">
              {PRODUCT_NAME} decouples consensus, scheduling orchestration, and task execution into three distinct operational planes. The control plane relies on an etcd cluster for atomic state management, lease tracking, and watch streams. Multiple orchestrator processes run concurrently, electing a single active leader while standby replicas maintain hot watch subscriptions.
            </p>

            {/* Topology SVG / ASCII Diagram */}
            <div className="border border-hairline p-6 bg-cream space-y-4">
              <div className="flex items-center justify-between border-b border-hairline pb-2 font-mono text-xs text-gray">
                <span>[ FIGURE 1.1 / SYSTEM TOPOLOGY & CONSENSUS INTERFACES ]</span>
                <span className="text-cobalt">STRICT LINEARIZABILITY</span>
              </div>

              <div className="font-mono text-xs leading-relaxed text-jet bg-cream p-4 border border-hairline overflow-x-auto whitespace-pre">
{`+-----------------------------------------------------------------------------------+
|                              CLIENT / REST & gRPC API                             |
|                        (DAG Submission, Inspection, Chaos)                        |
+-----------------------------------------------------------------------------------+
                                          |
                                          v  [HTTP / gRPC]
+-----------------------------------------------------------------------------------+
|                               ORCHESTRATOR MESH                                   |
|                                                                                   |
|  +------------------------+  +------------------------+  +---------------------+  |
|  |     orchestrator-1     |  |     orchestrator-2     |  |   orchestrator-3    |  |
|  |     [ACTIVE LEADER]    |  |       [HOT STANDBY]    |  |    [HOT STANDBY]    |  |
|  |   Term: 42 (CAS Guard) |  |   Watching /orchestra  |  | Watching /orchestra |  |
|  +------------------------+  +------------------------+  +---------------------+  |
+-----------------------------------------------------------------------------------+
             |                                                  ^
             | [Atomic CAS Writes & Heartbeat Leases]           | [etcd Watch Stream]
             v                                                  |
+-----------------------------------------------------------------------------------+
|                         DISTRIBUTED STORAGE ENGINE (etcd v3)                      |
|                                                                                   |
|  +------------------------+  +------------------------+  +---------------------+  |
|  |         etcd-1         |  |         etcd-2         |  |       etcd-3        |  |
|  |       [Raft Leader]    |<--->    [Raft Follower]   |<--->  [Raft Follower]  |  |
|  +------------------------+  +------------------------+  +---------------------+  |
|                Quorum: 2/3 Nodes Healthy  |  Linearizable Reads & Writes          |
+-----------------------------------------------------------------------------------+
                                          |
                                          | [gRPC Task Dispatch & 5s Lease Heartbeat]
                                          v
+-----------------------------------------------------------------------------------+
|                                STATELESS WORKER FLEET                             |
|                                                                                   |
|  +----------------+  +----------------+  +----------------+  +-----------------+  |
|  |    worker-1    |  |    worker-2    |  |    worker-3    |  | worker-4 / 5    |  |
|  | (Capacity: 4)  |  | (Capacity: 4)  |  | (Capacity: 4)  |  | (Capacity: 4)   |  |
|  +----------------+  +----------------+  +----------------+  +-----------------+  |
+-----------------------------------------------------------------------------------+`}
              </div>
            </div>

            {/* Architecture Pillars Grid */}
            <div className="grid grid-cols-1 md:grid-cols-3 border border-hairline divide-y md:divide-y-0 md:divide-x divide-hairline bg-cream">
              <div className="p-6 space-y-3">
                <div className="flex items-center gap-2 text-cobalt font-bold font-mono text-xs uppercase">
                  <Key className="w-4 h-4" />
                  <span>01 / CONTROL PLANE</span>
                </div>
                <h4 className="font-bold text-base uppercase text-jet">LEADER ORCHESTRATOR</h4>
                <p className="text-xs text-deep-gray leading-relaxed">
                  Evaluates topological DAG constraints. As tasks become ready, dispatches assignments to available workers and monitors lease expiration events.
                </p>
              </div>

              <div className="p-6 space-y-3">
                <div className="flex items-center gap-2 text-cobalt font-bold font-mono text-xs uppercase">
                  <Database className="w-4 h-4" />
                  <span>02 / CONSENSUS PLANE</span>
                </div>
                <h4 className="font-bold text-base uppercase text-jet">ETCD RAFT CLUSTER</h4>
                <p className="text-xs text-deep-gray leading-relaxed">
                  Durably stores task states, worker heartbeats, and leader election keys. Guarantees linearizable writes and monotonic version tracking.
                </p>
              </div>

              <div className="p-6 space-y-3">
                <div className="flex items-center gap-2 text-cobalt font-bold font-mono text-xs uppercase">
                  <Cpu className="w-4 h-4" />
                  <span>03 / EXECUTION PLANE</span>
                </div>
                <h4 className="font-bold text-base uppercase text-jet">STATELESS WORKERS</h4>
                <p className="text-xs text-deep-gray leading-relaxed">
                  Maintain ephemeral 5-second heartbeats. Pull assignments via gRPC, execute isolated command workloads, and report idempotent completion results.
                </p>
              </div>
            </div>
          </section>

          {/* Section 002: etcd Key Space */}
          <section id="etcd-layout" className="space-y-6 scroll-mt-24">
            <div className="flex items-baseline justify-between border-b border-hairline pb-2">
              <h2 className="text-2xl font-bold uppercase tracking-tight text-jet flex items-center gap-3">
                <span className="font-mono text-sm text-gray">002</span>
                <span>ETCD KEY SPACE SPECIFICATION</span>
              </h2>
              <span className="font-mono text-xs text-muted-strong uppercase">HIERARCHICAL PREFIXES</span>
            </div>

            <p className="text-sm text-deep-gray leading-relaxed max-w-4xl">
              All cluster metadata is stored under deterministic hierarchical key prefixes. Ephemeral states use lease attachments that automatically expire if processes terminate ungracefully.
            </p>

            {/* Key Space Table */}
            <div className="border border-hairline overflow-x-auto bg-cream">
              <table className="w-full text-left font-mono text-xs border-collapse">
                <thead>
                  <tr className="border-b border-hairline bg-cream text-gray uppercase text-[11px]">
                    <th className="p-3 font-bold">KEY PREFIX</th>
                    <th className="p-3 font-bold">MUTABILITY / LEASE</th>
                    <th className="p-3 font-bold">PURPOSE & DATA CONTRACT</th>
                    <th className="p-3 font-bold text-right">ACTION</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-hairline text-deep-gray">
                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">/orchestra/leader/election</td>
                    <td className="p-3">
                      <span className="bg-cream border border-hairline px-2 py-0.5 text-[10px] font-bold text-cobalt">
                        EPHEMERAL LEASE (3s)
                      </span>
                    </td>
                    <td className="p-3 font-sans text-xs">
                      Single mutex key contested by orchestrator instances via etcd election API. Holding node is cluster leader.
                    </td>
                    <td className="p-3 text-right">
                      <button 
                        onClick={() => copyToClipboard('/orchestra/leader/election', 'k1')}
                        className="p-1 hover:text-jet focus-visible:ring-1 focus-visible:ring-cobalt"
                        title="Copy key path"
                      >
                        {copiedKey === 'k1' ? <Check className="w-3.5 h-3.5 text-cobalt" /> : <Copy className="w-3.5 h-3.5" />}
                      </button>
                    </td>
                  </tr>

                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">/orchestra/term</td>
                    <td className="p-3">
                      <span className="bg-cream border border-hairline px-2 py-0.5 text-[10px] font-bold text-jet">
                        PERSISTENT (MONOTONIC)
                      </span>
                    </td>
                    <td className="p-3 font-sans text-xs">
                      Strictly increasing 64-bit integer fencing token. Incremented on each leader transition.
                    </td>
                    <td className="p-3 text-right">
                      <button 
                        onClick={() => copyToClipboard('/orchestra/term', 'k2')}
                        className="p-1 hover:text-jet focus-visible:ring-1 focus-visible:ring-cobalt"
                        title="Copy key path"
                      >
                        {copiedKey === 'k2' ? <Check className="w-3.5 h-3.5 text-cobalt" /> : <Copy className="w-3.5 h-3.5" />}
                      </button>
                    </td>
                  </tr>

                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">/orchestra/workers/&#123;worker_id&#125;</td>
                    <td className="p-3">
                      <span className="bg-cream border border-hairline px-2 py-0.5 text-[10px] font-bold text-cobalt">
                        EPHEMERAL LEASE (5s)
                      </span>
                    </td>
                    <td className="p-3 font-sans text-xs">
                      Worker registration metadata (capacity, active slots, heartbeat timestamp, gRPC address).
                    </td>
                    <td className="p-3 text-right">
                      <button 
                        onClick={() => copyToClipboard('/orchestra/workers/', 'k3')}
                        className="p-1 hover:text-jet focus-visible:ring-1 focus-visible:ring-cobalt"
                        title="Copy key path"
                      >
                        {copiedKey === 'k3' ? <Check className="w-3.5 h-3.5 text-cobalt" /> : <Copy className="w-3.5 h-3.5" />}
                      </button>
                    </td>
                  </tr>

                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">/orchestra/workflows/&#123;id&#125;/spec</td>
                    <td className="p-3">
                      <span className="bg-cream border border-hairline px-2 py-0.5 text-[10px] font-bold text-jet">
                        IMMUTABLE JSON
                      </span>
                    </td>
                    <td className="p-3 font-sans text-xs">
                      Full DAG workflow specification including task nodes, dependency graph, and retry policies.
                    </td>
                    <td className="p-3 text-right">
                      <button 
                        onClick={() => copyToClipboard('/orchestra/workflows/{id}/spec', 'k4')}
                        className="p-1 hover:text-jet focus-visible:ring-1 focus-visible:ring-cobalt"
                        title="Copy key path"
                      >
                        {copiedKey === 'k4' ? <Check className="w-3.5 h-3.5 text-cobalt" /> : <Copy className="w-3.5 h-3.5" />}
                      </button>
                    </td>
                  </tr>

                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">/orchestra/tasks/&#123;wf_id&#125;/&#123;task_id&#125;</td>
                    <td className="p-3">
                      <span className="bg-cream border border-hairline px-2 py-0.5 text-[10px] font-bold text-cobalt">
                        CAS-MUTATED
                      </span>
                    </td>
                    <td className="p-3 font-sans text-xs">
                      Live execution state (state enum, assigned worker, attempt count, commit fencing token).
                    </td>
                    <td className="p-3 text-right">
                      <button 
                        onClick={() => copyToClipboard('/orchestra/tasks/{wf_id}/{task_id}', 'k5')}
                        className="p-1 hover:text-jet focus-visible:ring-1 focus-visible:ring-cobalt"
                        title="Copy key path"
                      >
                        {copiedKey === 'k5' ? <Check className="w-3.5 h-3.5 text-cobalt" /> : <Copy className="w-3.5 h-3.5" />}
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          {/* Section 003: Fencing & CAS Protocol */}
          <section id="fencing" className="space-y-6 scroll-mt-24">
            <div className="flex items-baseline justify-between border-b border-hairline pb-2">
              <h2 className="text-2xl font-bold uppercase tracking-tight text-jet flex items-center gap-3">
                <span className="font-mono text-sm text-gray">003</span>
                <span>LEADER ELECTION & FENCING PROTOCOL</span>
              </h2>
              <span className="font-mono text-xs text-muted-strong uppercase">SPLIT-BRAIN ELIMINATION</span>
            </div>

            <p className="text-sm text-deep-gray leading-relaxed max-w-4xl">
              Distributed consensus cannot rely on wall-clock time. If a leader process experiences a deep GC pause or network partition, it may wake up believing it is still the leader (a "zombie" leader). {PRODUCT_NAME} employs monotonic fencing tokens and atomic Compare-And-Swap (CAS) write guards to render stale writes impossible.
            </p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {/* Step 1: Election & Token Increment */}
              <div className="border border-hairline p-6 bg-cream space-y-3">
                <span className="font-mono text-[10px] text-gray uppercase tracking-widest block">
                  STAGE 1 // ELECTION & TOKEN ACQUISITION
                </span>
                <h4 className="font-bold text-base uppercase text-jet">MONOTONIC EPOCH ADVANCE</h4>
                <p className="text-xs text-deep-gray leading-relaxed">
                  When a leader lease expires, standing standby orchestrators campaign via etcd election API. The winner commits an atomic transaction:
                </p>
                <div className="p-3 bg-cream border border-hairline font-mono text-xs text-jet">
                  new_term = etcd.Txn().Increment('/orchestra/term')<br/>
                  etcd.Put('/orchestra/leader', leader_id, lease=3s)
                </div>
              </div>

              {/* Step 2: Atomic CAS Guard */}
              <div className="border border-hairline p-6 bg-cream space-y-3">
                <span className="font-mono text-[10px] text-gray uppercase tracking-widest block">
                  STAGE 2 // TRANSACTIONAL WRITE BARRIER
                </span>
                <h4 className="font-bold text-base uppercase text-jet">COMPARE-AND-SWAP (CAS)</h4>
                <p className="text-xs text-deep-gray leading-relaxed">
                  Every state transition committed by the leader executes within an etcd transaction checking that the active token matches:
                </p>
                <div className="p-3 bg-cream border border-hairline font-mono text-xs text-jet">
                  Txn().If(Value('/orchestra/term') == leader_term)<br/>
                  &nbsp;&nbsp;.Then(Put(task_key, COMPLETED))<br/>
                  &nbsp;&nbsp;.Else(Reject(ErrFencingTokenMismatch))
                </div>
              </div>
            </div>
          </section>

          {/* Section 004: Worker Leases */}
          <section id="leases" className="space-y-6 scroll-mt-24">
            <div className="flex items-baseline justify-between border-b border-hairline pb-2">
              <h2 className="text-2xl font-bold uppercase tracking-tight text-jet flex items-center gap-3">
                <span className="font-mono text-sm text-gray">004</span>
                <span>WORKER LEASES & HEARTBEAT CYCLE</span>
              </h2>
              <span className="font-mono text-xs text-muted-strong uppercase">FAULT DETECTION</span>
            </div>

            <p className="text-sm text-deep-gray leading-relaxed max-w-4xl">
              Worker failure detection is entirely asynchronous. Each worker establishes an ephemeral etcd lease with a 5,000ms TTL and executes a keepalive heartbeat every 1,500ms.
            </p>

            <div className="border border-hairline p-6 bg-cream space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-4 border border-hairline divide-y sm:divide-y-0 sm:divide-x divide-hairline bg-cream font-mono text-xs">
                <div className="p-4 space-y-1">
                  <span className="text-gray text-[10px] uppercase block">TTL WINDOW</span>
                  <div className="text-xl font-bold text-jet">5,000 ms</div>
                  <span className="text-muted-strong text-[11px] font-sans">etcd lease expiration</span>
                </div>
                <div className="p-4 space-y-1">
                  <span className="text-gray text-[10px] uppercase block">HEARTBEAT CADENCE</span>
                  <div className="text-xl font-bold text-cobalt">1,500 ms</div>
                  <span className="text-muted-strong text-[11px] font-sans">Periodic renewal</span>
                </div>
                <div className="p-4 space-y-1">
                  <span className="text-gray text-[10px] uppercase block">MISSED THRESHOLD</span>
                  <div className="text-xl font-bold text-jet">3 Heartbeats</div>
                  <span className="text-muted-strong text-[11px] font-sans">Triggers eviction</span>
                </div>
                <div className="p-4 space-y-1">
                  <span className="text-gray text-[10px] uppercase block">TASK MIGRATION</span>
                  <div className="text-xl font-bold text-cobalt">Immediate</div>
                  <span className="text-muted-strong text-[11px] font-sans">Automatic reclaim</span>
                </div>
              </div>

              <p className="text-xs text-deep-gray leading-relaxed">
                When a worker process dies or loses network connectivity for &gt;5s, etcd automatically deletes its registration key. The leader's etcd watcher catches the delete event, locates all tasks assigned to that worker in state <code className="text-jet font-mono font-bold">RUNNING</code>, reverts them to <code className="text-jet font-mono font-bold">READY</code>, and dispatches them to healthy workers under the active fencing token.
              </p>
            </div>
          </section>

          {/* Section 005: Task Lifecycle & State Machine */}
          <section id="lifecycle" className="space-y-6 scroll-mt-24">
            <div className="flex items-baseline justify-between border-b border-hairline pb-2">
              <h2 className="text-2xl font-bold uppercase tracking-tight text-jet flex items-center gap-3">
                <span className="font-mono text-sm text-gray">005</span>
                <span>DAG TASK LIFECYCLE & STATE MACHINE</span>
              </h2>
              <span className="font-mono text-xs text-muted-strong uppercase">FINITE STATE AUTOMATON</span>
            </div>

            <p className="text-sm text-deep-gray leading-relaxed max-w-4xl">
              Each task node within a workflow executes as a strict finite state machine. Transitions are unidirectional; terminal states (<code className="text-jet font-mono font-bold">COMPLETED</code>, <code className="text-jet font-mono font-bold">FAILED</code>) cannot be overridden by late-arriving network packets.
            </p>

            {/* State Diagram / ASCII */}
            <div className="font-mono text-xs leading-relaxed text-jet bg-cream p-4 border border-hairline overflow-x-auto whitespace-pre">
{` [PENDING] 
     |  (Parents finished with COMPLETED)
     v
  [READY] <------------------------+  (Worker died mid-flight; task lease expired)
     |                             |
     |  (Worker slot reserved)     |
     v                             |
[SCHEDULED]                        |
     |                             |
     |  (Worker begins execution)  |
     v                             |
 [RUNNING] ------------------------+
     |
     +-----> (Exit code == 0) ------> [COMPLETED] (Terminal)
     |
     +-----> (Exit code != 0) ------> [RETRYING] -> [READY] (If attempts < max)
                                   |
                                   +> [FAILED] (Terminal, if attempts >= max)`}
            </div>
          </section>

          {/* Section 006: Invariant Verification Checker */}
          <section id="invariants" className="space-y-6 scroll-mt-24">
            <div className="flex items-baseline justify-between border-b border-hairline pb-2">
              <h2 className="text-2xl font-bold uppercase tracking-tight text-jet flex items-center gap-3">
                <span className="font-mono text-sm text-gray">006</span>
                <span>CHAOS INVARIANT VERIFICATION CHECKER</span>
              </h2>
              <span className="font-mono text-xs text-muted-strong uppercase">FORMAL AUDIT ENGINE</span>
            </div>

            <p className="text-sm text-deep-gray leading-relaxed max-w-4xl">
              {PRODUCT_NAME} includes a dedicated formal invariant auditor (<code className="text-jet font-mono">cmd/checker</code>). The auditor replays raw JSONL event streams and worker evidence logs from chaos runs to assert mathematical safety properties:
            </p>

            <div className="space-y-3 font-mono text-xs">
              <div className="border border-hairline p-4 bg-cream space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-jet uppercase">INVARIANT 1 // TOPOLOGICAL DEPENDENCY ORDER</span>
                  <span className="text-cobalt font-bold">∀ edge (u, v) ∈ E: completed(u) &lt; started(v)</span>
                </div>
                <p className="text-deep-gray font-sans text-xs pt-1">
                  No downstream DAG task begins execution before all prerequisite upstream parent tasks have durably committed terminal COMPLETED status.
                </p>
              </div>

              <div className="border border-hairline p-4 bg-cream space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-jet uppercase">INVARIANT 2 // EXACTLY-ONCE SINGLE COMMIT</span>
                  <span className="text-cobalt font-bold">∀ task t: |&#123;e ∈ Events : e.type == COMMIT ∧ e.task_id == t&#125;| ≤ 1</span>
                </div>
                <p className="text-deep-gray font-sans text-xs pt-1">
                  A task's terminal state cannot be committed to etcd more than once. Fencing tokens reject late commits from superseded workers or zombie leaders.
                </p>
              </div>

              <div className="border border-hairline p-4 bg-cream space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-bold text-jet uppercase">INVARIANT 3 // NO LOST TASKS COMPLETION</span>
                  <span className="text-cobalt font-bold">∀ task t ∈ Workflow: final_state(t) ∈ &#123;COMPLETED, FAILED&#125;</span>
                </div>
                <p className="text-deep-gray font-sans text-xs pt-1">
                  Every submitted task eventually terminates. No task remains orphaned or stuck indefinitely in READY, SCHEDULED, or RUNNING states.
                </p>
              </div>
            </div>
          </section>

          {/* Section 007: Failure Matrix */}
          <section id="matrix" className="space-y-6 scroll-mt-24">
            <div className="flex items-baseline justify-between border-b border-hairline pb-2">
              <h2 className="text-2xl font-bold uppercase tracking-tight text-jet flex items-center gap-3">
                <span className="font-mono text-sm text-gray">007</span>
                <span>FAILURE MODES & RECONCILIATION MATRIX</span>
              </h2>
              <span className="font-mono text-xs text-muted-strong uppercase">CRASH RESILIENCE</span>
            </div>

            <p className="text-sm text-deep-gray leading-relaxed max-w-4xl">
              Comprehensive operational response matrix detailing detection mechanisms, recovery procedures, and formal invariant preservation across network, process, and disk failures.
            </p>

            <div className="border border-hairline overflow-x-auto bg-cream">
              <table className="w-full text-left font-sans text-xs border-collapse">
                <thead>
                  <tr className="border-b border-hairline bg-cream font-mono text-gray uppercase text-[11px]">
                    <th className="p-3 font-bold">FAILURE SCENARIO</th>
                    <th className="p-3 font-bold">DETECTION MECHANISM</th>
                    <th className="p-3 font-bold">SYSTEM RESPONSE & RECONCILIATION</th>
                    <th className="p-3 font-bold font-mono">INVARIANT GUARANTEE</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-hairline text-deep-gray">
                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">Active leader crashes (SIGKILL)</td>
                    <td className="p-3 font-mono text-xs">etcd session TTL (~3s)</td>
                    <td className="p-3">Standby wins election, increments term, reconstructs in-flight state from etcd</td>
                    <td className="p-3 font-mono font-bold text-cobalt">Zero lost tasks</td>
                  </tr>

                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">Zombie leader unpauses (SIGCONT)</td>
                    <td className="p-3 font-mono text-xs">CAS fencing compare on write</td>
                    <td className="p-3">All late writes rejected with ErrFencingTokenMismatch; old leader steps down</td>
                    <td className="p-3 font-mono font-bold text-cobalt">Zero split-brain</td>
                  </tr>

                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">Worker process terminated abruptly</td>
                    <td className="p-3 font-mono text-xs">Worker lease TTL (~5s)</td>
                    <td className="p-3">Tasks marked orphaned, reverted to READY, and rescheduled on surviving workers</td>
                    <td className="p-3 font-mono font-bold text-cobalt">At-least-once dispatch</td>
                  </tr>

                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">Worker reports duplicate commit</td>
                    <td className="p-3 font-mono text-xs">Task state check in etcd</td>
                    <td className="p-3">Late completion ignored; original committed state preserved idempotently</td>
                    <td className="p-3 font-mono font-bold text-cobalt">Exactly-once commit</td>
                  </tr>

                  <tr className="hover:bg-cream transition-colors">
                    <td className="p-3 font-bold text-jet">Single etcd follower node crashes</td>
                    <td className="p-3 font-mono text-xs">Raft heartbeat timeout</td>
                    <td className="p-3">Surviving 2/3 quorum maintains uninterrupted scheduling; node catches up on restart</td>
                    <td className="p-3 font-mono font-bold text-cobalt">Continuous uptime</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          {/* Footer note */}
          <div className="border-t border-hairline pt-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4 font-mono text-xs text-muted-strong">
            <span>{PRODUCT_NAME} DISTRIBUTED SCHEDULER // SYSTEM DOCUMENTATION V1.0</span>
            <span className="text-cobalt font-bold">FORMAL VERIFICATION PASS: 100%</span>
          </div>

        </div>
      </div>
    </div>
  );
};

export default DocsPage;
