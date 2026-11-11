import { Suspense } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { ClusterProvider } from './context/ClusterContext';
import { ToastProvider } from './components/ui/Toast';
import { AppShell } from './components/layout/AppShell';
import { Skeleton } from './components/ui/Skeleton';

import LandingPage from './pages/LandingPage';
import ClusterPage from './pages/ClusterPage';
import WorkflowsPage from './pages/WorkflowsPage';
import SubmitPage from './pages/SubmitPage';
import ResiliencePage from './pages/ResiliencePage';
import DocsPage from './pages/DocsPage';
import NotFound from './pages/NotFound';

function PageLoader() {
  return (
    <div className="flex-1 p-6 max-w-7xl mx-auto w-full space-y-4">
      <Skeleton height="3.5rem" />
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Skeleton height="6rem" />
        <Skeleton height="6rem" />
        <Skeleton height="6rem" />
        <Skeleton height="6rem" />
      </div>
      <Skeleton height="20rem" />
    </div>
  );
}

export default function App() {
  return (
    <ToastProvider>
      <ClusterProvider>
        <Suspense fallback={<PageLoader />}>
          <Routes>
            {/* Public Landing Page (No App Shell) */}
            <Route path="/" element={<LandingPage />} />

            {/* Application Shell Routes */}
            <Route path="/app" element={<AppShell />}>
              <Route index element={<ClusterPage />} />
              <Route path="workflows" element={<WorkflowsPage />} />
              <Route path="workflows/:id" element={<WorkflowsPage />} />
              <Route path="submit" element={<SubmitPage />} />
              <Route path="resilience" element={<ResiliencePage />} />
              <Route path="docs" element={<DocsPage />} />
            </Route>

            {/* Legacy redirect for backwards compatibility */}
            <Route path="/cluster" element={<Navigate to="/app" replace />} />

            {/* Helpful 404 Route */}
            <Route path="*" element={<NotFound />} />
          </Routes>
        </Suspense>
      </ClusterProvider>
    </ToastProvider>
  );
}
