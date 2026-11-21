import { Controller, Get, ServiceUnavailableException } from '@nestjs/common';
import { DataSource } from 'typeorm';

// Readiness-style health check: 200 only if the API can actually reach
// Postgres, since an API that's "up" but can't read trips is useless to
// the mobile app's "backend online" indicator. Kept dependency-free
// (no @nestjs/terminus) because there's only one thing to check.
@Controller('health')
export class HealthController {
  constructor(private readonly dataSource: DataSource) {}

  @Get()
  async check() {
    try {
      await this.dataSource.query('SELECT 1');
    } catch {
      throw new ServiceUnavailableException({ status: 'error', db: 'down' });
    }
    return { status: 'ok', db: 'up' };
  }
}
