import React, { useState, useMemo, useEffect, useCallback } from 'react';
import { Link } from 'react-router-dom';
import ReactFlow, {
  Edge,
  Controls,
  Background,
  useNodesState,
  useEdgesState,
} from 'reactflow';
import 'reactflow/dist/style.css';
import dagre from 'dagre';
import { DagTaskNode } from './DagTaskNode';
import { 
  WorkflowWithTasks, 
  Task, 
  TaskState, 
} from '../types';
import { 
  Search, 
  Copy, 
  Check, 
  RotateCw, 
  X,
  ArrowRight,
  Plus
} from 'lucide-react';
import { StatusBadge } from './ui/StatusBadge';
import { PosterButton } from './ui/PosterButton';
import { Tabs } from './ui/Tabs';
import { ConfirmDialog } from './ui/ConfirmDialog';
import { useToast } from './ui/Toast';

interface WorkflowDetailViewProps {
  workflows: WorkflowWithTasks[];
  selectedWfId: string | null;
  onSelectWorkflow: (id: string) => void;
  onCancelWorkflow: (id: string) => Promise<void>;
  onRefresh: () => void;
}

const nodeTypes = {
  taskNode: DagTaskNode,
};

const NODE_WIDTH = 230;
const NODE_HEIGHT = 100;

export const WorkflowDetailView: React.FC<WorkflowDetailViewProps> = ({
  workflows,
  selectedWfId,
  onSelectWorkflow,
  onCancelWorkflow,
  onRefresh,
}) => {
  const { showToast } = useToast();
  const [activeTab, setActiveTab] = useState<'graph' | 'score' | 'timeline' | 'definition'>('graph');
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [filterState, setFilterState] = useState<string>('ALL');
  const [searchQuery, setSearchQuery] = useState('');
  const [confirmCancelOpen, setConfirmCancelOpen] = useState(false);
  const [copiedDef, setCopiedDef] = useState(false);

  // Active workflow selection
  const currentWf = useMemo(() => {
    if (!selectedWfId && workflows.length > 0) {
      return workflows[0];
    }
    return workflows.find(w => w.workflow?.spec?.id === selectedWfId) || workflows[0] || null;
  }, [workflows, selectedWfId]);

  // Filtered workflows for list
  const filteredWorkflows = useMemo(() => {
    return workflows.filter(w => {
      const spec = w.workflow?.spec;
      const status = w.workflow?.status;
      const matchSearch = !searchQuery || 
        (spec?.name?.toLowerCase().includes(searchQuery.toLowerCase()) || 
         spec?.id?.toLowerCase().includes(searchQuery.toLowerCase()));

      if (!matchSearch) return false;
      if (filterState === 'ALL') return true;

      const stateNum = status?.state;
      if (filterState === 'RUNNING') return stateNum === 1;
      if (filterState === 'COMPLETED') return stateNum === 2;
      if (filterState === 'FAILED') return stateNum === 4;
      if (filterState === 'CANCELLED') return stateNum === 3;
      return true;
    });
  }, [workflows, searchQuery, filterState]);

  // Dagre layout computation for Graph tab
  const { initialNodes, initialEdges } = useMemo(() => {
    if (!currentWf || !currentWf.tasks || currentWf.tasks.length === 0) {
      return { initialNodes: [], initialEdges: [] };
    }

    const g = new dagre.graphlib.Graph();
    g.setDefaultEdgeLabel(() => ({}));
    g.setGraph({ rankdir: 'LR', nodesep: 50, ranksep: 80 });

    const tasks = currentWf.tasks;
    tasks.forEach(t => {
      g.setNode(t.spec.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
    });

    const edges: Edge[] = [];
    tasks.forEach(t => {
      (t.spec.dependencies || []).forEach(depId => {
        g.setEdge(depId, t.spec.id);
        edges.push({
          id: `e-${depId}-${t.spec.id}`,
          source: depId,
          target: t.spec.id,
          type: 'smoothstep',
          style: { stroke: '#141414', strokeWidth: 1.5 },
        });
      });
    });

    dagre.layout(g);

    const nodes = tasks.map(t => {
      const nodePos = g.node(t.spec.id) || { x: 0, y: 0 };
      return {
        id: t.spec.id,
        type: 'taskNode',
        data: { task: t, isSelected: selectedTask?.spec.id === t.spec.id },
        position: { x: nodePos.x - NODE_WIDTH / 2, y: nodePos.y - NODE_HEIGHT / 2 },
      };
    });

    return { initialNodes: nodes, initialEdges: edges };
  }, [currentWf, selectedTask]);

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  useEffect(() => {
    setNodes(initialNodes);
    setEdges(initialEdges);
  }, [initialNodes, initialEdges, setNodes, setEdges]);

  const onNodeClick = useCallback((_: any, node: any) => {
    const task = currentWf?.tasks?.find(t => t.spec.id === node.id);
    if (task) setSelectedTask(task);
  }, [currentWf]);

  const handleCancel = async () => {
    if (!currentWf) return;
    try {
      await onCancelWorkflow(currentWf.workflow.spec.id);
      showToast('Workflow cancelled');
    } finally {
      setConfirmCancelOpen(false);
    }
  };

  const handleCopyDefinition = () => {
    if (!currentWf) return;
    const jsonStr = JSON.stringify(currentWf, null, 2);
    navigator.clipboard.writeText(jsonStr);
    setCopiedDef(true);
    showToast('Definition copied to clipboard');
    setTimeout(() => setCopiedDef(false), 2500);
  };

  const tabs = [
    { id: 'graph', label: 'GRAPH' },
    { id: 'score', label: 'SCORE' },
    { id: 'timeline', label: 'TIMELINE' },
    { id: 'definition', label: 'DEFINITION' },
  ];

  if (!currentWf) {
    return (
      <div className="p-8 sm:p-12 text-center space-y-4">
        <h2 className="font-display font-bold uppercase text-2xl text-jet">NO WORKFLOWS FOUND</h2>
        <p className="text-sm text-deep-gray">Submit a new workflow definition to begin execution.</p>
        <Link to="/app/submit">
          <PosterButton variant="primary">SUBMIT WORKFLOW</PosterButton>
        </Link>
      </div>
    );
  }

  const wfSpec = currentWf.workflow?.spec || { id: 'unknown', name: 'Unknown' };
  const wfStatus = currentWf.workflow?.status || { state: 1, total_tasks: 0, completed_tasks: 0 };
  const doneTasks = wfStatus.completed_tasks || 0;
  const totalTasks = wfStatus.total_tasks || currentWf.tasks?.length || 1;
  const pct = Math.round((doneTasks / totalTasks) * 100);

  return (
    <div className="flex flex-col w-full font-mono">
      {/* 1. TOP MASTER BAR / WORKFLOW SELECTOR & FILTERS */}
      <section className="border-b border-hairline p-6 lg:p-8 space-y-6">
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
          <div className="space-y-1">
            <span className="font-mono text-xs text-muted">[ WORKFLOWS DATABASE ]</span>
            <h1 className="font-display font-black text-2xl sm:text-4xl uppercase tracking-tighter text-jet">
              {wfSpec.name || wfSpec.id}
            </h1>
          </div>

          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={onRefresh}
              className="p-2 border border-hairline hover:bg-jet hover:text-cream transition-colors duration-300 focus-visible:outline-none"
              title="Refresh workflows"
            >
              <RotateCw className="h-4 w-4" />
            </button>

            <Link to="/app/submit">
              <PosterButton variant="primary" size="sm">
                <Plus className="h-3.5 w-3.5 mr-1" />
                SUBMIT WORKFLOW
              </PosterButton>
            </Link>
          </div>
        </div>

        {/* Search & State Filter Toggles */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-t border-hairline pt-4">
          <div className="flex items-center gap-2 overflow-x-auto text-xs uppercase font-bold">
            {['ALL', 'RUNNING', 'COMPLETED', 'FAILED', 'CANCELLED'].map(state => (
              <button
                key={state}
                onClick={() => setFilterState(state)}
                className={`px-3 py-1 border transition-colors duration-300 focus-visible:outline-none ${
                  filterState === state
                    ? 'border-cobalt bg-cobalt text-cream'
                    : 'border-hairline text-muted hover:text-jet'
                }`}
              >
                {state}
              </button>
            ))}
          </div>

          <div className="relative">
            <Search className="absolute left-3 top-2.5 h-3.5 w-3.5 text-muted pointer-events-none" />
            <input
              type="text"
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              placeholder="SEARCH WORKFLOWS..."
              className="pl-9 pr-3 py-1.5 font-mono text-xs border border-hairline bg-cream text-jet placeholder:text-muted focus-visible:outline-none w-56"
            />
          </div>
        </div>
      </section>

      {/* 2. DETAIL METADATA STRIP & ACTIONS */}
      <section className="border-b border-hairline grid grid-cols-2 lg:grid-cols-4 bg-cream">
        <div className="p-4 sm:p-6 border-r border-b lg:border-b-0 border-hairline space-y-1">
          <span className="text-[10px] text-muted uppercase">WORKFLOW ID</span>
          <div className="font-bold text-sm text-jet truncate">{wfSpec.id}</div>
        </div>

        <div className="p-4 sm:p-6 border-r border-b lg:border-b-0 border-hairline space-y-1">
          <span className="text-[10px] text-muted uppercase">STATUS</span>
          <div>
            <StatusBadge state={wfStatus.state} label={wfStatus.state === 2 ? 'COMPLETED' : wfStatus.state === 3 ? 'CANCELLED' : 'RUNNING'} />
          </div>
        </div>

        <div className="p-4 sm:p-6 border-r border-hairline space-y-1">
          <span className="text-[10px] text-muted uppercase">PROGRESS</span>
          <div className="flex items-center gap-2">
            <div className="flex-1 h-2 bg-cream border border-hairline overflow-hidden">
              <div className="h-full bg-cobalt transition-all duration-300" style={{ width: `${pct}%` }} />
            </div>
            <span className="text-xs text-jet font-bold">{doneTasks}/{totalTasks}</span>
          </div>
        </div>

        <div className="p-4 sm:p-6 flex items-center justify-between">
          <div className="space-y-1">
            <span className="text-[10px] text-muted uppercase">ACTIONS</span>
            <div>
              {wfStatus.state === 1 ? (
                <button
                  type="button"
                  onClick={() => setConfirmCancelOpen(true)}
                  className="text-xs text-red hover:underline font-bold focus-visible:outline-none"
                >
                  CANCEL WORKFLOW
                </button>
              ) : (
                <span className="text-xs text-muted">TERMINAL STATE</span>
              )}
            </div>
          </div>
        </div>
      </section>

      {/* 3. FOUR DETAIL TABS */}
      <section className="border-b border-hairline px-6 lg:px-8 bg-cream">
        <Tabs
          tabs={tabs}
          activeTab={activeTab}
          onChange={(t) => setActiveTab(t as any)}
        />
      </section>

      {/* 4. TAB PANELS */}
      <div className="flex-1 min-h-[500px]">
        {/* TAB 1: GRAPH (ReactFlow + Dagre with flat square nodes) */}
        {activeTab === 'graph' && (
          <div className="grid grid-cols-12 min-h-[550px] relative">
            <div className={`col-span-12 ${selectedTask ? 'lg:col-span-8' : 'col-span-12'} h-[550px] bg-cream border-r border-hairline relative`}>
              <ReactFlow
                nodes={nodes}
                edges={edges}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                onNodeClick={onNodeClick}
                nodeTypes={nodeTypes}
                fitView
                fitViewOptions={{ padding: 0.3 }}
              >
                <Background color="#C7C7C7" gap={24} size={1} />
                <Controls className="!border !border-hairline !bg-cream !rounded-none !shadow-none" />
              </ReactFlow>
            </div>

            {/* Task Details Side Inspector Panel */}
            {selectedTask && (
              <aside className="col-span-12 lg:col-span-4 p-6 bg-cream border-t lg:border-t-0 border-hairline space-y-6">
                <div className="flex items-start justify-between border-b border-hairline pb-4">
                  <div className="space-y-1">
                    <span className="font-mono text-xs text-muted">[ TASK INSPECTOR ]</span>
                    <h3 className="font-display font-bold uppercase text-lg text-jet">
                      {selectedTask.spec.id}
                    </h3>
                  </div>
                  <button
                    type="button"
                    onClick={() => setSelectedTask(null)}
                    className="p-1 hover:text-cobalt focus-visible:outline-none"
                  >
                    <X className="h-4 w-4" />
                  </button>
                </div>

                <div className="space-y-3 font-mono text-xs">
                  <div className="flex justify-between border-b border-hairline pb-2">
                    <span className="text-muted">STATE:</span>
                    <StatusBadge state={selectedTask.status.state} />
                  </div>
                  <div className="flex justify-between border-b border-hairline pb-2">
                    <span className="text-muted">ASSIGNED WORKER:</span>
                    <span className="font-bold text-jet">{selectedTask.status.assigned_worker || 'None'}</span>
                  </div>
                  <div className="flex justify-between border-b border-hairline pb-2">
                    <span className="text-muted">ATTEMPT:</span>
                    <span>{selectedTask.status.current_attempt || 1} / {selectedTask.spec.max_retries || 2}</span>
                  </div>
                  <div className="flex justify-between border-b border-hairline pb-2">
                    <span className="text-muted">TIMEOUT:</span>
                    <span>{selectedTask.spec.timeout_seconds || 20}s</span>
                  </div>
                  <div className="space-y-1 border-b border-hairline pb-2">
                    <span className="text-muted">DEPENDENCIES:</span>
                    <div className="flex flex-wrap gap-1 mt-1">
                      {(selectedTask.spec.dependencies || []).length === 0 ? (
                        <span className="text-muted-strong">[ Root node - no parent deps ]</span>
                      ) : (
                        (selectedTask.spec.dependencies || []).map(d => (
                          <span key={d} className="px-1.5 py-0.2 border border-hairline bg-cream font-bold">
                            {d}
                          </span>
                        ))
                      )}
                    </div>
                  </div>
                  <div className="space-y-1">
                    <span className="text-muted">PAYLOAD:</span>
                    <pre className="p-2 border border-hairline bg-cream text-[11px] overflow-x-auto whitespace-pre-wrap">
                      {selectedTask.spec.payload || 'No payload definition'}
                    </pre>
                  </div>
                </div>
              </aside>
            )}
          </div>
        )}

        {/* TAB 2: SCORE (Workflow-specific timeline) */}
        {activeTab === 'score' && (
          <div className="p-6 lg:p-12 space-y-6">
            <div className="space-y-1 border-b border-hairline pb-4">
              <span className="text-xs text-muted">[ TASK DISPATCH STAVES ]</span>
              <h3 className="font-display font-bold uppercase text-xl text-jet">
                Workflow Lane Allocations
              </h3>
            </div>

            <div className="border border-hairline divide-y divide-hairline">
              {currentWf.tasks.map(t => {
                const assigned = (t.status.assigned_worker || 'Unassigned').toLowerCase();
                const isDone = t.status.state === TaskState.COMPLETED;
                const isRun = t.status.state === TaskState.RUNNING;

                return (
                  <div key={t.spec.id} className="p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4 hover:bg-[rgba(255,255,255,0.2)]">
                    <div className="space-y-1">
                      <div className="flex items-center gap-2">
                        <span className="font-bold text-sm text-jet">{t.spec.id}</span>
                        <StatusBadge state={t.status.state} />
                      </div>
                      <div className="text-xs text-muted">
                        Assigned to: <strong className="text-jet">{assigned}</strong> (Attempt {t.status.current_attempt || 1})
                      </div>
                    </div>

                    <div className="w-full sm:w-64 h-6 border border-hairline relative bg-cream">
                      <div
                        className={`h-full ${isDone ? 'bg-jet' : isRun ? 'bg-cobalt animate-pulse' : 'bg-transparent'}`}
                        style={{ width: isDone ? '100%' : isRun ? '60%' : '0%' }}
                      />
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* TAB 3: TIMELINE (Chronological trace with explanations) */}
        {activeTab === 'timeline' && (
          <div className="p-6 lg:p-12 space-y-6 font-mono text-xs">
            <div className="space-y-1 border-b border-hairline pb-4">
              <span className="text-xs text-muted">[ CHRONOLOGICAL TRACE ]</span>
              <h3 className="font-display font-bold uppercase text-xl text-jet">
                Execution &amp; Failover History
              </h3>
            </div>

            <div className="border border-hairline divide-y divide-hairline bg-cream">
              <div className="p-4 flex items-baseline gap-4 hover:bg-[rgba(255,255,255,0.2)]">
                <span className="text-muted min-w-[5rem]">T+00.0s</span>
                <span className="bg-jet text-cream px-1.5 py-0.2 font-bold">DISPATCHED</span>
                <span className="text-deep-gray">Workflow graph admitted; initial root tasks marked READY.</span>
              </div>
              <div className="p-4 flex items-baseline gap-4 hover:bg-[rgba(255,255,255,0.2)]">
                <span className="text-muted min-w-[5rem]">T+01.5s</span>
                <span className="bg-cobalt text-cream px-1.5 py-0.2 font-bold">SCHEDULED</span>
                <span className="text-deep-gray">Task fetch dispatched to worker-1 under active 5s heartbeat lease.</span>
              </div>
              <div className="p-4 flex items-baseline gap-4 hover:bg-[rgba(255,255,255,0.2)]">
                <span className="text-muted min-w-[5rem]">T+05.2s</span>
                <span className="bg-jet text-cream px-1.5 py-0.2 font-bold">COMPLETED</span>
                <span className="text-deep-gray">Task fetch committed result to etcd via CAS token version 1.</span>
              </div>
              <div className="p-4 flex items-baseline gap-4 hover:bg-[rgba(255,255,255,0.2)]">
                <span className="text-muted min-w-[5rem]">T+05.5s</span>
                <span className="bg-cobalt text-cream px-1.5 py-0.2 font-bold">BARRIER</span>
                <span className="text-deep-gray">Parent barrier satisfied. Branch tasks transformA and transformB released.</span>
              </div>
              <div className="p-4 flex items-baseline gap-4 hover:bg-[rgba(255,255,255,0.2)]">
                <span className="text-muted min-w-[5rem]">T+08.0s</span>
                <span className="bg-red text-cream px-1.5 py-0.2 font-bold">RE-QUEUE</span>
                <span className="text-deep-gray">worker-3 halted mid-run; task transformB vacated and re-queued to worker-1 without duplicate commit.</span>
              </div>
              <div className="p-4 flex items-baseline gap-4 hover:bg-[rgba(255,255,255,0.2)]">
                <span className="text-muted min-w-[5rem]">T+12.8s</span>
                <span className="bg-jet text-cream px-1.5 py-0.2 font-bold">IDEMPOTENT</span>
                <span className="text-deep-gray">Late result packet from severed worker-3 dropped safely by CAS generation barrier.</span>
              </div>
            </div>
          </div>
        )}

        {/* TAB 4: DEFINITION (Copyable YAML/JSON) */}
        {activeTab === 'definition' && (
          <div className="p-6 lg:p-12 space-y-6 font-mono text-xs">
            <div className="flex items-center justify-between border-b border-hairline pb-4">
              <div className="space-y-1">
                <span className="text-xs text-muted">[ SPECIFICATION DEFINITION ]</span>
                <h3 className="font-display font-bold uppercase text-xl text-jet">
                  Declarative DAG Schema
                </h3>
              </div>
              <button
                type="button"
                onClick={handleCopyDefinition}
                className="flex items-center gap-1.5 px-3 py-1.5 border border-hairline hover:bg-jet hover:text-cream transition-colors duration-300 focus-visible:outline-none"
              >
                {copiedDef ? <Check className="h-3.5 w-3.5 text-cobalt" /> : <Copy className="h-3.5 w-3.5" />}
                <span>{copiedDef ? 'COPIED' : 'COPY DEFINITION'}</span>
              </button>
            </div>

            <div className="border border-hairline p-4 bg-cream overflow-x-auto">
              <pre className="text-xs text-jet leading-relaxed">
                {JSON.stringify(currentWf, null, 2)}
              </pre>
            </div>
          </div>
        )}
      </div>

      {/* 5. WORKFLOWS REGISTRY TABLE (Hairline Table) */}
      <section className="border-t border-hairline p-6 lg:p-12 space-y-6">
        <div className="space-y-1 border-b border-hairline pb-4">
          <span className="font-mono text-xs text-muted">[ ALL WORKFLOWS ]</span>
          <h2 className="font-display font-bold text-xl uppercase tracking-tight text-jet">
            Execution Registry
          </h2>
        </div>

        <div className="overflow-x-auto border border-hairline">
          <table className="w-full text-left text-xs border-collapse font-mono">
            <thead>
              <tr className="border-b border-hairline bg-cream font-bold uppercase text-muted text-[11px]">
                <th className="py-3 px-4">Status</th>
                <th className="py-3 px-4">Workflow Name / ID</th>
                <th className="py-3 px-4">Started</th>
                <th className="py-3 px-4">Progress (Tasks Done/Total)</th>
                <th className="py-3 px-4 text-right">Action</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-hairline">
              {filteredWorkflows.map((wf) => {
                const s = wf.workflow?.spec;
                const st = wf.workflow?.status;
                const isSelected = s?.id === currentWf?.workflow?.spec?.id;
                const d = st?.completed_tasks || 0;
                const tot = st?.total_tasks || wf.tasks?.length || 1;
                const p = Math.round((d / tot) * 100);

                return (
                  <tr
                    key={s?.id}
                    onClick={() => onSelectWorkflow(s?.id || '')}
                    className={`cursor-pointer transition-colors duration-300 ${
                      isSelected ? 'bg-[rgba(255,255,255,0.4)] font-bold' : 'hover:bg-[rgba(255,255,255,0.2)]'
                    }`}
                  >
                    <td className="py-3 px-4">
                      <StatusBadge state={st?.state} label={st?.state === 2 ? 'DONE' : st?.state === 3 ? 'CANCEL' : 'RUN'} />
                    </td>
                    <td className="py-3 px-4">
                      <div className="font-bold text-jet">{s?.name || s?.id}</div>
                      <div className="text-[10px] text-muted">{s?.id}</div>
                    </td>
                    <td className="py-3 px-4 text-muted-strong">
                      {st?.created_at ? new Date(st.created_at).toLocaleTimeString() : 'Recent'}
                    </td>
                    <td className="py-3 px-4 w-48">
                      <div className="flex items-center gap-2">
                        <div className="flex-1 h-2 bg-cream border border-hairline overflow-hidden">
                          <div className="h-full bg-cobalt transition-all duration-300" style={{ width: `${p}%` }} />
                        </div>
                        <span className="text-[11px] text-muted-strong">{d}/{tot}</span>
                      </div>
                    </td>
                    <td className="py-3 px-4 text-right">
                      <button
                        type="button"
                        onClick={(e) => {
                          e.stopPropagation();
                          onSelectWorkflow(s?.id || '');
                        }}
                        className="inline-flex items-center gap-1 text-xs text-cobalt hover:underline focus-visible:outline-none"
                      >
                        <span>INSPECT</span>
                        <ArrowRight className="h-3 w-3" />
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </section>

      {/* Confirmed Cancel Dialog */}
      <ConfirmDialog
        isOpen={confirmCancelOpen}
        title="CANCEL WORKFLOW EXECUTION?"
        message={`Are you sure you want to cancel workflow ${wfSpec.id}? In-flight tasks will be halted and the workflow marked as CANCELLED.`}
        confirmText="CANCEL WORKFLOW"
        confirmVariant="danger"
        onConfirm={handleCancel}
        onCancel={() => setConfirmCancelOpen(false)}
      />
    </div>
  );
};

export default WorkflowDetailView;
