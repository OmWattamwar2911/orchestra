import { useState, useEffect, useRef, useCallback } from 'react';
import { EventRecord } from '../types';
import { clusterClient } from './client';

export type SSEConnectionStatus = 'connected' | 'reconnecting' | 'disconnected';

export interface UseEventStreamOptions {
  onEvent?: (event: EventRecord) => void;
  maxStoredEvents?: number;
}

export function useEventStream({ onEvent, maxStoredEvents = 150 }: UseEventStreamOptions = {}) {
  const [status, setStatus] = useState<SSEConnectionStatus>('connected');
  const [events, setEvents] = useState<EventRecord[]>([]);
  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;

  const handleIncomingEvent = useCallback((event: EventRecord) => {
    setEvents(prev => [event, ...prev.slice(0, maxStoredEvents - 1)]);
    if (onEventRef.current) {
      onEventRef.current(event);
    }
  }, [maxStoredEvents]);

  useEffect(() => {
    // If simulator, simulate connection
    if (clusterClient.isSimulator()) {
      setStatus('connected');
      const unsubscribe = clusterClient.subscribeEvents(handleIncomingEvent);
      return () => unsubscribe();
    }

    let es: EventSource | null = null;
    let reconnectTimeout: any = null;
    let backoff = 1000;
    let unmounted = false;

    const connect = () => {
      if (unmounted) return;
      setStatus('reconnecting');
      try {
        es = new EventSource('/api/events');

        es.onopen = () => {
          if (unmounted) return;
          setStatus('connected');
          backoff = 1000;
        };

        es.addEventListener('connected', () => {
          if (unmounted) return;
          setStatus('connected');
        });

        const eventTypes = ['task_event', 'worker_event', 'leader_event'];
        eventTypes.forEach(type => {
          es?.addEventListener(type, (e: MessageEvent) => {
            if (unmounted) return;
            try {
              const data: EventRecord = JSON.parse(e.data);
              handleIncomingEvent(data);
            } catch {}
          });
        });

        es.onerror = () => {
          if (unmounted) return;
          setStatus('reconnecting');
          if (es) {
            es.close();
            es = null;
          }
          reconnectTimeout = setTimeout(() => {
            backoff = Math.min(backoff * 1.5, 8000);
            connect();
          }, backoff);
        };
      } catch {
        if (unmounted) return;
        setStatus('disconnected');
        reconnectTimeout = setTimeout(connect, 3000);
      }
    };

    connect();

    return () => {
      unmounted = true;
      if (es) es.close();
      if (reconnectTimeout) clearTimeout(reconnectTimeout);
    };
  }, [handleIncomingEvent]);

  const clearEvents = useCallback(() => {
    setEvents([]);
  }, []);

  return {
    status,
    events,
    clearEvents,
    setEvents,
  };
}
