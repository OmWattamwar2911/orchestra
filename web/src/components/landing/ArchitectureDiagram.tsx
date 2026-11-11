import React from 'react';

export const ArchitectureDiagram: React.FC = () => {
  return (
    <div className="w-full space-y-6">
      <div className="border border-hairline p-6 sm:p-10 bg-cream">
        {/* SVG Flat Line Drawing */}
        <div className="w-full overflow-x-auto">
          <svg
            viewBox="0 0 900 380"
            fill="none"
            xmlns="http://www.w3.org/2000/svg"
            className="w-full min-w-[700px] h-auto font-mono text-xs"
          >
            {/* ETCD STATE STORE (Top Center) */}
            <rect x="330" y="20" width="240" height="70" stroke="#141414" strokeWidth="1.5" fill="#E3E2DE" />
            <text x="450" y="48" textAnchor="middle" fill="#141414" fontWeight="bold" fontSize="13" fontFamily="General Sans, sans-serif">
              ETCD CONSENSUS STORE
            </text>
            <text x="450" y="68" textAnchor="middle" fill="#7A7A7A" fontSize="10">
              Raft Log • Fencing Tokens • TTL Leases
            </text>

            {/* ORCHESTRATOR NODES (Middle Row) */}
            {/* Node 1: Leader */}
            <rect x="60" y="150" width="220" height="90" stroke="#1351AA" strokeWidth="2" fill="#E3E2DE" />
            <text x="170" y="178" textAnchor="middle" fill="#1351AA" fontWeight="bold" fontSize="13" fontFamily="General Sans, sans-serif">
              ORCHESTRATOR-1 [LEADER]
            </text>
            <text x="170" y="198" textAnchor="middle" fill="#141414" fontSize="11">
              Active Term 1 (Token = 1)
            </text>
            <text x="170" y="218" textAnchor="middle" fill="#5C5C5C" fontSize="10">
              DAG Dispatcher &amp; Barrier Scheduler
            </text>

            {/* Node 2: Standby */}
            <rect x="340" y="150" width="220" height="90" stroke="#141414" strokeWidth="1.5" fill="#E3E2DE" />
            <text x="450" y="178" textAnchor="middle" fill="#141414" fontWeight="bold" fontSize="13" fontFamily="General Sans, sans-serif">
              ORCHESTRATOR-2 [STANDBY]
            </text>
            <text x="450" y="198" textAnchor="middle" fill="#7A7A7A" fontSize="11">
              Watching etcd leader lease
            </text>
            <text x="450" y="218" textAnchor="middle" fill="#5C5C5C" fontSize="10">
              Ready for immediate failover
            </text>

            {/* Node 3: Standby */}
            <rect x="620" y="150" width="220" height="90" stroke="#141414" strokeWidth="1.5" fill="#E3E2DE" />
            <text x="730" y="178" textAnchor="middle" fill="#141414" fontWeight="bold" fontSize="13" fontFamily="General Sans, sans-serif">
              ORCHESTRATOR-3 [STANDBY]
            </text>
            <text x="730" y="198" textAnchor="middle" fill="#7A7A7A" fontSize="11">
              Watching etcd leader lease
            </text>
            <text x="730" y="218" textAnchor="middle" fill="#5C5C5C" fontSize="10">
              Replicating state machine
            </text>

            {/* WORKERS FLEET (Bottom Row) */}
            <rect x="60" y="290" width="780" height="70" stroke="#141414" strokeWidth="1.5" fill="#E3E2DE" />
            <text x="450" y="318" textAnchor="middle" fill="#141414" fontWeight="bold" fontSize="13" fontFamily="General Sans, sans-serif">
              WORKER FLEET (worker-1 through worker-5)
            </text>
            <text x="450" y="338" textAnchor="middle" fill="#5C5C5C" fontSize="10">
              5-Second Heartbeat Leases • gRPC Task Stream • Local Goroutine Pool
            </text>

            {/* CONNECTIONS & ANNOTATIONS */}
            {/* etcd to Leader */}
            <line x1="170" y1="150" x2="380" y2="90" stroke="#1351AA" strokeWidth="1.5" strokeDasharray="3 3" />
            <text x="230" y="115" fill="#1351AA" fontSize="10" fontWeight="bold">
              CAS Token Mutation
            </text>

            {/* etcd to Standby 2 */}
            <line x1="450" y1="150" x2="450" y2="90" stroke="#7A7A7A" strokeWidth="1" strokeDasharray="2 2" />
            <text x="460" y="125" fill="#7A7A7A" fontSize="10">
              Watch Key
            </text>

            {/* etcd to Standby 3 */}
            <line x1="730" y1="150" x2="520" y2="90" stroke="#7A7A7A" strokeWidth="1" strokeDasharray="2 2" />

            {/* Leader to Workers */}
            <line x1="170" y1="240" x2="170" y2="290" stroke="#1351AA" strokeWidth="2" />
            <text x="180" y="270" fill="#1351AA" fontSize="10" fontWeight="bold">
              gRPC Dispatch
            </text>

            {/* Workers to etcd (Heartbeats) */}
            <path d="M 810 290 L 810 55 L 570 55" stroke="#7A7A7A" strokeWidth="1" strokeDasharray="3 3" />
            <text x="720" y="45" fill="#7A7A7A" fontSize="10">
              TTL Lease Renewal
            </text>
          </svg>
        </div>

        {/* Captions Grid */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 pt-8 border-t border-hairline mt-6 font-sans">
          <div className="space-y-1">
            <h4 className="font-mono text-xs font-bold uppercase text-cobalt">[ 01 / ELECTION ]</h4>
            <p className="text-xs text-deep-gray leading-relaxed">
              Orchestrators run an etcd election campaign with an ephemeral lease. The winner becomes leader and increments the cluster term.
            </p>
          </div>

          <div className="space-y-1">
            <h4 className="font-mono text-xs font-bold uppercase text-jet">[ 02 / FENCING ]</h4>
            <p className="text-xs text-deep-gray leading-relaxed">
              Every workflow state write requires a compare-and-swap (CAS) check against the fencing token. Stale zombie leaders cannot commit corrupt writes.
            </p>
          </div>

          <div className="space-y-1">
            <h4 className="font-mono text-xs font-bold uppercase text-jet">[ 03 / LEASES ]</h4>
            <p className="text-xs text-deep-gray leading-relaxed">
              Workers continuously heartbeat every 2 seconds. When a worker dies, its 5-second lease expires, and the leader migrates tasks without loss.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ArchitectureDiagram;
