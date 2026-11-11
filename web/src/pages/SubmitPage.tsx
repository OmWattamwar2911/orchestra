import React from 'react';
import { useNavigate } from 'react-router-dom';
import { SubmitView } from '../components/SubmitView';
import { useCluster } from '../context/ClusterContext';
import { useToast } from '../components/ui/Toast';

export const SubmitPage: React.FC = () => {
  const navigate = useNavigate();
  const { refresh } = useCluster();
  const { showToast } = useToast();

  const handleSuccess = async (id: string) => {
    showToast(`Workflow submitted: ${id}`);
    await refresh();
    navigate('/app/workflows');
  };

  return (
    <div className="flex-1 flex flex-col w-full">
      <SubmitView onSubmitSuccess={handleSuccess} />
    </div>
  );
};

export default SubmitPage;
