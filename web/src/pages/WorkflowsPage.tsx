import React, { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useCluster } from '../context/ClusterContext';
import { WorkflowDetailView } from '../components/WorkflowDetailView';
import { Skeleton } from '../components/ui/Skeleton';
import { ErrorState } from '../components/ui/ErrorState';

export const WorkflowsPage: React.FC = () => {
  const { id } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const { workflows, loading, error, refresh, cancelWorkflow } = useCluster();
  
  const [selectedWfId, setSelectedWfId] = useState<string | null>(id || null);

  useEffect(() => {
    if (id) {
      setSelectedWfId(id);
    } else if (workflows.length > 0 && !selectedWfId) {
      setSelectedWfId(workflows[0].workflow?.spec?.id || null);
    }
  }, [id, workflows, selectedWfId]);

  const handleSelect = (workflowId: string) => {
    setSelectedWfId(workflowId);
    navigate(`/app/workflows/${workflowId}`);
  };

  const activeId = selectedWfId || (workflows.length > 0 ? workflows[0].workflow?.spec?.id || null : null);

  if (loading && workflows.length === 0) {
    return (
      <div className="p-6 max-w-7xl mx-auto space-y-4">
        <Skeleton height="3rem" />
        <Skeleton height="20rem" />
      </div>
    );
  }

  if (error && workflows.length === 0) {
    return (
      <div className="p-6 max-w-3xl mx-auto">
        <ErrorState
          title="Failed to load workflows"
          message={error}
          onRetry={refresh}
        />
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col">
      <WorkflowDetailView
        workflows={workflows}
        selectedWfId={activeId}
        onSelectWorkflow={handleSelect}
        onCancelWorkflow={cancelWorkflow}
        onRefresh={refresh}
      />
    </div>
  );
};

export default WorkflowsPage;
