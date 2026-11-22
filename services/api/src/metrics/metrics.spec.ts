import { EventEmitter } from 'node:events';
import type { NextFunction, Request, Response } from 'express';
import { httpMetricsMiddleware, httpRequestDuration } from './metrics';

async function countFor(labels: Record<string, string>): Promise<number> {
  const metric = await httpRequestDuration.get();
  const sample = metric.values.find(
    (v) =>
      v.metricName === 'tracklah_http_request_duration_seconds_count' &&
      Object.entries(labels).every(([k, val]) => v.labels[k] === val),
  );
  return sample?.value ?? 0;
}

function run(req: Partial<Request>, status: number): void {
  const res = Object.assign(new EventEmitter(), { statusCode: status });
  const next = jest.fn() as NextFunction;
  httpMetricsMiddleware(req as Request, res as unknown as Response, next);
  expect(next).toHaveBeenCalled();
  res.emit('finish');
}

describe('httpMetricsMiddleware', () => {
  it('labels a request with its route template, not the raw URL', async () => {
    const labels = { method: 'GET', route: '/trips/:id', status: '200' };
    const before = await countFor(labels);
    run(
      {
        method: 'GET',
        baseUrl: '',
        originalUrl: '/trips/11111111-1111-4111-8111-111111111111',
        route: { path: '/trips/:id' },
      },
      200,
    );
    expect((await countFor(labels)) - before).toBe(1);
  });

  it('puts requests that matched no route under "unmatched"', async () => {
    const labels = { method: 'GET', route: 'unmatched', status: '404' };
    const before = await countFor(labels);
    run({ method: 'GET', baseUrl: '', originalUrl: '/wp-login.php' }, 404);
    expect((await countFor(labels)) - before).toBe(1);
  });
});
