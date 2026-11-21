import { join } from 'node:path';
import { DataSourceOptions } from 'typeorm';
import { Trip } from '../trips/trip.entity';

type Env = Record<string, string | undefined>;

// One source of truth for the TypeORM connection, shared by the Nest app
// (app.module.ts) and the migration CLI (data-source.ts), so the two can't
// drift apart.
//
// Schema changes go through migrations only. synchronize stays off: on a
// shared Postgres it could silently drop columns.
export function typeOrmOptions(env: Env): DataSourceOptions {
  return {
    type: 'postgres',
    // gen_random_uuid() is built into Postgres 13+, so the migrations don't
    // need CREATE EXTENSION (uuid-ossp), which a non-superuser app role
    // may not be allowed to run.
    uuidExtension: 'pgcrypto',
    host: env.DB_HOST,
    port: Number(env.DB_PORT ?? 5432),
    username: env.DB_USER,
    password: env.DB_PASSWORD,
    database: env.DB_NAME,
    entities: [Trip],
    migrations: [join(__dirname, 'migrations', '*.{js,ts}')],
    synchronize: false,
    // Pending migrations are applied when the API boots. Fine for a single
    // API instance; with several replicas this should move to a separate
    // deploy step.
    migrationsRun: env.DB_MIGRATIONS_RUN !== 'false',
  };
}
