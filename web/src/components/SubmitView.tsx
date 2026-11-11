import React, { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { validateWorkflowText, ValidationResult } from '../utils/dagValidator';
import { useCluster } from '../context/ClusterContext';
import { useToast } from './ui/Toast';
import { PosterButton } from './ui/PosterButton';
import { CheckCircle2, AlertCircle } from 'lucide-react';

interface SubmitViewProps {
  onSubmitSuccess?: (newWfId: string) => void;
}

const SAMPLE_TEMPLATES: Record<string, { name: string; yaml: string }> = {
  diamond: {
    name: '1. DIAMOND DAG (Classic Fork-Join)',
    yaml: `# Classic diamond fork-join workflow
id: diamond-pipeline-\${TIMESTAMP}
name: "Diamond Invariant Pipeline"
failure_policy: FAIL_FAST
tasks:
  - id: fetch
    type: sleep
    payload: "sleep: 2s"
    timeout_seconds: 20
    max_retries: 2
    dependencies: []
  - id: transformA
    type: sleep
    payload: "sleep: 3s"
    timeout_seconds: 20
    dependencies:
      - fetch
  - id: transformB
    type: sleep
    payload: "sleep: 4s"
    timeout_seconds: 20
    dependencies:
      - fetch
  - id: merge
    type: sleep
    payload: "sleep: 2s"
    timeout_seconds: 20
    dependencies:
      - transformA
      - transformB
  - id: publish
    type: sleep
    payload: "sleep: 1s"
    timeout_seconds: 20
    dependencies:
      - merge
`,
  },

  fanout: {
    name: '2. PARALLEL FAN-OUT (Multi-Worker)',
    yaml: `# Wide parallel fan-out across worker fleet
id: fanout-pipeline-\${TIMESTAMP}
name: "Parallel Sharded Fan-Out"
failure_policy: FAIL_FAST
tasks:
  - id: split_source
    type: sleep
    payload: "partition 4 shards"
    timeout_seconds: 20
    dependencies: []
  - id: worker_shard_1
    type: sleep
    payload: "sleep: 1s"
    timeout_seconds: 20
    dependencies:
      - split_source
  - id: worker_shard_2
    type: sleep
    payload: "sleep: 1.5s"
    timeout_seconds: 20
    dependencies:
      - split_source
  - id: worker_shard_3
    type: sleep
    payload: "sleep: 1.2s"
    timeout_seconds: 20
    dependencies:
      - split_source
  - id: worker_shard_4
    type: sleep
    payload: "sleep: 1.8s"
    timeout_seconds: 20
    dependencies:
      - split_source
  - id: merge_aggregates
    type: sleep
    payload: "aggregate results"
    timeout_seconds: 20
    dependencies:
      - worker_shard_1
      - worker_shard_2
      - worker_shard_3
      - worker_shard_4
`,
  },

  failfast: {
    name: '3. FAULT INJECTION (Fail-Fast Retry)',
    yaml: `# Demonstrates worker crash and lease re-queue
id: fault-recovery-\${TIMESTAMP}
name: "Fault Injection DAG"
failure_policy: FAIL_FAST
tasks:
  - id: ingest
    type: sleep
    payload: "sleep: 1s"
    timeout_seconds: 20
    dependencies: []
  - id: compute_flaky
    type: sleep
    payload: "sleep: 3s"
    timeout_seconds: 20
    max_retries: 3
    dependencies:
      - ingest
  - id: commit_sink
    type: sleep
    payload: "sleep: 1s"
    timeout_seconds: 20
    dependencies:
      - compute_flaky
`,
  },
};

export const SubmitView: React.FC<SubmitViewProps> = ({ onSubmitSuccess }) => {
  const navigate = useNavigate();
  const { submitWorkflow, refresh } = useCluster();
  const { showToast } = useToast();

  const [selectedTemplateKey, setSelectedTemplateKey] = useState('diamond');
  const [editorText, setEditorText] = useState('');
  const [validation, setValidation] = useState<ValidationResult>({ valid: true });
  const [submitting, setSubmitting] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  const loadTemplate = (key: string) => {
    const tmpl = SAMPLE_TEMPLATES[key];
    if (tmpl) {
      const populated = tmpl.yaml.replace(/\$\{TIMESTAMP\}/g, String(Date.now()).slice(-4));
      setEditorText(populated);
      setSelectedTemplateKey(key);
      setServerError(null);
    }
  };

  useEffect(() => {
    loadTemplate('diamond');
  }, []);

  useEffect(() => {
    const res = validateWorkflowText(editorText);
    setValidation(res);
  }, [editorText]);

  // Extract task list for live visual preview
  const previewTasks = useMemo(() => {
    if (!validation.workflow || !validation.workflow.tasks) return [];
    return validation.workflow.tasks;
  }, [validation]);

  // Detailed inline error diagnostic with line numbers
  const inlineDiagnostic = useMemo(() => {
    if (validation.valid) return null;

    const lines = editorText.split('\n');
    let lineNum = 1;
    let remedyAdvice = 'Review task dependencies and ensure all IDs exist without circular loops.';

    if (validation.error) {
      const errLower = validation.error.toLowerCase();
      // Try to match task ID mentioned in error
      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (line.includes('id:') || line.includes('- id:')) {
          const parts = line.split('id:');
          const id = parts[1]?.trim().replace(/['"]/g, '');
          if (id && errLower.includes(id.toLowerCase())) {
            lineNum = i + 1;
            break;
          }
        }
      }

      if (errLower.includes('missing') || errLower.includes('depends')) {
        remedyAdvice = 'A task depends on a parent that does not exist in this graph. Add the missing task definition or remove the dependency.';
      } else if (errLower.includes('cycle') || errLower.includes('circular')) {
        remedyAdvice = 'Circular dependency detected. Invert or remove the dependency edge to preserve strict DAG acyclicity.';
      }
    }

    return {
      line: lineNum,
      message: validation.error || 'Syntax or topological structure invalid',
      remedy: remedyAdvice,
    };
  }, [validation, editorText]);

  const handleSubmit = async () => {
    if (!validation.valid || submitting) return;
    setSubmitting(true);
    setServerError(null);

    try {
      const res = await submitWorkflow(editorText);
      showToast('Workflow submitted');
      if (onSubmitSuccess) {
        onSubmitSuccess(res.workflow_id);
      }
      await refresh();
      navigate('/app/workflows');
    } catch (err: any) {
      setServerError(`Submission rejected: ${err.message || 'Verification failed'}. Check cluster health and try again.`);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex flex-col w-full font-mono">
      {/* 1. TOP HEADER & TEMPLATE SELECTOR */}
      <section className="border-b border-hairline p-6 lg:p-8 space-y-4 bg-cream">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="space-y-1">
            <span className="text-xs text-muted uppercase">[ DAG DISPATCH ENGINE ]</span>
            <h1 className="font-display font-black text-2xl sm:text-4xl uppercase tracking-tighter text-jet">
              Submit Workflow Definition
            </h1>
          </div>

          <div className="flex items-center gap-3">
            <label htmlFor="sample-picker" className="text-xs text-muted font-bold uppercase">
              Sample:
            </label>
            <select
              id="sample-picker"
              value={selectedTemplateKey}
              onChange={e => loadTemplate(e.target.value)}
              className="border border-hairline bg-cream px-3 py-2 text-xs font-mono font-bold text-jet uppercase focus-visible:outline-none cursor-pointer"
            >
              {Object.entries(SAMPLE_TEMPLATES).map(([k, v]) => (
                <option key={k} value={k}>
                  {v.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        <p className="font-sans text-xs text-deep-gray max-w-2xl leading-relaxed">
          Author declarative task graphs in YAML or JSON. Orchestra validates topological acyclicity, calculates dependency barriers, and assigns monotonic fencing tokens before scheduling across workers.
        </p>
      </section>

      {/* 2. SPLIT VIEW: EDITOR LEFT, LIVE DAG PREVIEW RIGHT (Split by a hairline) */}
      <div className="grid grid-cols-1 lg:grid-cols-2 divide-y lg:divide-y-0 lg:divide-x divide-hairline min-h-[600px]">
        {/* LEFT PANE: CODE EDITOR & INLINE ERROR DIAGNOSTICS */}
        <div className="flex flex-col justify-between p-6 lg:p-8 space-y-6 bg-cream">
          <div className="space-y-4">
            <div className="flex items-center justify-between border-b border-hairline pb-2">
              <span className="text-xs text-muted">[ 01 / SPECIFICATION EDITOR ]</span>
              <span className="text-[11px] text-muted">YAML / JSON</span>
            </div>

            {/* Monospace Code Editor Area */}
            <div className="relative border border-hairline bg-cream">
              <textarea
                value={editorText}
                onChange={e => setEditorText(e.target.value)}
                rows={18}
                spellCheck={false}
                className="w-full resize-y bg-cream p-4 font-mono text-xs leading-relaxed text-jet placeholder:text-muted focus-visible:outline-none"
                placeholder="Paste workflow YAML definition here..."
                aria-label="Workflow YAML or JSON code"
              />
            </div>

            {/* Plain Inline Errors with Line Numbers */}
            {inlineDiagnostic && (
              <div className="border-2 border-red bg-cream p-4 space-y-2">
                <div className="flex items-center gap-2 font-bold text-red text-xs">
                  <AlertCircle className="h-4 w-4" />
                  <span>LINE {inlineDiagnostic.line}: {inlineDiagnostic.message}</span>
                </div>
                <p className="font-sans text-xs text-deep-gray leading-relaxed pl-6">
                  {inlineDiagnostic.remedy}
                </p>
              </div>
            )}

            {serverError && (
              <div className="border border-red bg-cream p-4 text-xs text-red font-bold">
                [ SERVER ERROR ]: {serverError}
              </div>
            )}
          </div>

          {/* Submission Bar */}
          <div className="border-t border-hairline pt-6 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div className="text-xs text-muted font-sans">
              Topological levels calculated before commit.
            </div>

            <PosterButton
              variant="primary"
              onClick={handleSubmit}
              disabled={!validation.valid || submitting}
              loading={submitting}
            >
              SUBMIT WORKFLOW
            </PosterButton>
          </div>
        </div>

        {/* RIGHT PANE: LIVE DAG PREVIEW */}
        <div className="p-6 lg:p-8 space-y-6 bg-cream flex flex-col justify-between">
          <div className="space-y-6">
            <div className="flex items-center justify-between border-b border-hairline pb-2">
              <span className="text-xs text-muted">[ 02 / LIVE GRAPH TOPOLOGY ]</span>
              <div>
                {validation.valid ? (
                  <span className="inline-flex items-center gap-1.5 px-2 py-0.5 border border-cobalt text-cobalt font-bold text-[11px] uppercase">
                    <CheckCircle2 className="h-3.5 w-3.5" />
                    <span>VALID DAG ({validation.taskCount || previewTasks.length} TASKS)</span>
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 px-2 py-0.5 border border-red text-red font-bold text-[11px] uppercase">
                    <AlertCircle className="h-3.5 w-3.5" />
                    <span>INVALID TOPOLOGY</span>
                  </span>
                )}
              </div>
            </div>

            {/* Live Visual Graph Layout */}
            {previewTasks.length === 0 ? (
              <div className="py-20 text-center text-xs text-muted border border-hairline">
                [ WAITING FOR VALID TASK DEFINITIONS ]
              </div>
            ) : (
              <div className="space-y-3">
                <div className="text-xs text-muted-strong font-sans">
                  Execution order computed by dependency levels:
                </div>

                <div className="space-y-2">
                  {previewTasks.map((t, idx) => {
                    const deps = t.dependencies || [];
                    const isRoot = deps.length === 0;

                    return (
                      <div
                        key={t.id || idx}
                        className="border border-hairline p-3 flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-cream hover:bg-[rgba(255,255,255,0.3)] transition-colors duration-300"
                      >
                        <div className="flex items-center gap-3">
                          <span className="text-xs text-muted font-mono">{idx + 1 < 10 ? `0${idx + 1}` : idx + 1}</span>
                          <span className="font-bold text-jet uppercase text-xs sm:text-sm">{t.id}</span>
                          <span className="text-[10px] text-muted uppercase">[{t.type || 'sleep'}]</span>
                        </div>

                        <div className="flex items-center gap-2 text-xs">
                          {isRoot ? (
                            <span className="px-1.5 py-0.2 bg-cobalt text-cream font-bold text-[10px] uppercase">
                              ROOT BARRIER
                            </span>
                          ) : (
                            <div className="flex items-center gap-1 text-muted-strong text-[11px]">
                              <span>Awaits:</span>
                              {deps.map(d => (
                                <span key={d} className="px-1 py-0.2 border border-hairline font-bold text-jet">
                                  {d}
                                </span>
                              ))}
                            </div>
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>

          {/* Bottom Invariant Guarantee Card */}
          <div className="border border-hairline p-4 font-mono text-xs space-y-1 bg-cream mt-8">
            <div className="text-cobalt font-bold">[ INVARIANT GUARANTEE ]</div>
            <p className="font-sans text-xs text-deep-gray leading-relaxed">
              No task will ever be assigned to a worker until every upstream dependency reports successful terminal commitment.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};

export default SubmitView;
