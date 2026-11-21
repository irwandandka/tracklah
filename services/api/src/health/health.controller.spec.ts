import { ServiceUnavailableException } from '@nestjs/common';
import { DataSource } from 'typeorm';
import { HealthController } from './health.controller';

describe('HealthController', () => {
  it('reports ok when the database answers', async () => {
    const ds = { query: jest.fn().mockResolvedValue([{ '?column?': 1 }]) };
    const controller = new HealthController(ds as unknown as DataSource);
    await expect(controller.check()).resolves.toEqual({
      status: 'ok',
      db: 'up',
    });
  });

  it('returns 503 when the database is unreachable', async () => {
    const ds = {
      query: jest.fn().mockRejectedValue(new Error('ECONNREFUSED')),
    };
    const controller = new HealthController(ds as unknown as DataSource);
    await expect(controller.check()).rejects.toBeInstanceOf(
      ServiceUnavailableException,
    );
  });
});
