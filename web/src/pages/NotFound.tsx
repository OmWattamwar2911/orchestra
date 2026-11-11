import React from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Compass } from 'lucide-react';
import { Button } from '../components/ui/Button';
import { PRODUCT_NAME } from '../config';

export const NotFound: React.FC = () => {
  return (
    <div className="min-h-screen bg-canvas text-text-primary flex flex-col items-center justify-center p-6 text-center">
      <div className="max-w-md w-full space-y-6">
        <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-control bg-surface-elevated border border-border-default text-accent shadow-sm">
          <Compass className="h-7 w-7" aria-hidden="true" />
        </div>

        <div className="space-y-2">
          <div className="font-mono text-xs font-semibold text-accent">Error 404</div>
          <h1 className="font-display text-2xl font-bold tracking-tight text-text-primary">
            Page not found
          </h1>
          <p className="text-sm text-text-secondary">
            The destination you requested does not exist or was moved. Check the URL or return to the {PRODUCT_NAME} cluster overview.
          </p>
        </div>

        <div className="flex flex-col sm:flex-row items-center justify-center gap-3 pt-2">
          <Link to="/app" className="w-full sm:w-auto">
            <Button variant="primary" className="w-full sm:w-auto">
              <ArrowLeft className="h-4 w-4 mr-1.5" />
              Go to cluster overview
            </Button>
          </Link>
          <Link to="/" className="w-full sm:w-auto">
            <Button variant="secondary" className="w-full sm:w-auto">
              View landing page
            </Button>
          </Link>
        </div>
      </div>
    </div>
  );
};

export default NotFound;
