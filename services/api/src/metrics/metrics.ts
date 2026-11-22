import { createServer, type Server } from 'node:http';
import type { NextFunction, Request, Response } from 'express';
import {
  Counter,
  Histogram,
  Registry,
  collectDefaultMetrics,
} from 'prom-client';

// One registry for the whole process. Prometheus scrapes it from a
// separate port (see startMetricsServer), so /metrics never goes through
// the public Cloudflare Tunnel that fronts the API port.
export const registry = new Registry();
collectDefaultMetrics({ register: registry });

export const httpRequestDuration = new Histogram({
  name: 'tracklah_http_request_duration_seconds',
  help: 'HTTP request latency, by route template and status code',
  labelNames: ['method', 'route', 'status'] as const,
  buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5],
  registers: [registry],
});

export const tripsCreated = new Counter({
  name: 'tracklah_trips_created_total',
  help: 'Trips created through POST /trips',
  registers: [registry],
});

export const tripTransitions = new Counter({
  name: 'tracklah_trip_transitions_total',
  help: 'Trip status changes, by new status and why (cancels only)',
  labelNames: ['status', 'reason'] as const,
  registers: [registry],
});

// Times a request and records it under its route template ("/trips/:id",
// never the raw URL), so label cardinality stays bounded. Requests that
// match no route (404s, scanners) all land in "unmatched".
export function httpMetricsMiddleware(
  req: Request,
  res: Response,
  next: NextFunction,
): void {
  const end = httpRequestDuration.startTimer();
  res.on('finish', () => {
    const route = (req.route as { path?: string } | undefined)?.path;
    end({
      method: req.method,
      route: route ? `${req.baseUrl}${route}` : 'unmatched',
      status: String(res.statusCode),
    });
  });
  next();
}

export function startMetricsServer(port: number): Server {
  return createServer((req, res) => {
    if (req.method !== 'GET' || req.url !== '/metrics') {
      res.statusCode = 404;
      res.end();
      return;
    }
    registry
      .metrics()
      .then((body) => {
        res.setHeader('Content-Type', registry.contentType);
        res.end(body);
      })
      .catch((err: unknown) => {
        res.statusCode = 500;
        res.end(String(err));
      });
  }).listen(port);
}
