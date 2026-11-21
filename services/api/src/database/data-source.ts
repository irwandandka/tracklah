import { DataSource } from 'typeorm';
import { typeOrmOptions } from './typeorm-options';

// Used by the TypeORM CLI only (npm run migration:*). Env comes from
// `node --env-file=.env`, see package.json.
export default new DataSource(typeOrmOptions(process.env));
