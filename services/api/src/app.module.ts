import { Module } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { TypeOrmModule } from '@nestjs/typeorm';
import { ScheduleModule } from '@nestjs/schedule';
import { ThrottlerGuard, ThrottlerModule } from '@nestjs/throttler';
import { APP_GUARD } from '@nestjs/core';
import { AppController } from './app.controller';
import { AppService } from './app.service';
import { TripsModule } from './trips/trips.module';
import { CommandsModule } from './commands/commands.module';
import { LocationModule } from './location/location.module';
import { HealthModule } from './health/health.module';
import { typeOrmOptions } from './database/typeorm-options';
import { clientIp } from './throttle/client-ip';

@Module({
  imports: [
    ConfigModule.forRoot({ isGlobal: true }),
    ScheduleModule.forRoot(),
    // The API has no auth yet, so this is the main guard against one client
    // hammering it. Limits are per client IP (see clientIp), in memory, so
    // they reset on restart and aren't shared between replicas.
    ThrottlerModule.forRoot({
      throttlers: [
        {
          name: 'default',
          ttl: 60_000,
          limit: Number(process.env.THROTTLE_LIMIT_PER_MINUTE ?? 120),
        },
      ],
      getTracker: (req) =>
        clientIp(
          req as Parameters<typeof clientIp>[0],
          process.env.TRUST_CF_CONNECTING_IP === 'true',
        ),
    }),
    TypeOrmModule.forRootAsync({
      useFactory: () => typeOrmOptions(process.env),
    }),
    TripsModule,
    CommandsModule,
    LocationModule,
    HealthModule,
  ],
  controllers: [AppController],
  providers: [AppService, { provide: APP_GUARD, useClass: ThrottlerGuard }],
})
export class AppModule {}
