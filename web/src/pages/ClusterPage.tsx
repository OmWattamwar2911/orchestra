import React from 'react';
import { useCluster } from '../context/ClusterContext';
import { ClusterView } from '../components/ClusterView';
import { Skeleton } from '../components/ui/Skeleton';
import { ErrorState } from '../components/ui/ErrorState';

export const ClusterPage: React.FC = () => {
  const { cluster, workflows, events, clearEvents, loading, error, refresh } = useCluster();

  if (loading && !cluster) {
    return (
      <div className="p-6 lg:p-12 space-y-6">
        <Skeleton height="4rem" />
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-0 border border-hairline">
          <Skeleton height="7rem" />
          <Skeleton height="7rem" />
          <Skeleton height="7rem" />
          <Skeleton height="7rem" />
        </div>
        <Skeleton height="18rem" />
      </div>
    );
  }

  if (error && !cluster) {
    return (
      <div className="p-6 lg:p-12 max-w-2xl">
        <ErrorState
          title="FAILED TO CONNECT TO CLUSTER"
          message={error}
          onRetry={refresh}
        />
      </div>
    );
  }

  if (!cluster) {
    return null;
  }

  return (
    <ClusterView
      cluster={cluster}
      workflows={workflows}
      events={events}
      onClearEvents={clearEvents}
    />
  );
};

export default ClusterPage;
