import {
  Injectable,
  Logger,
  OnModuleDestroy,
  OnModuleInit,
} from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import Redis from 'ioredis';

const CHANNEL = 'location-updates';

// Subscribing needs a dedicated connection - once an ioredis client
// issues SUBSCRIBE, that connection can't be used for anything else, so
// this deliberately does NOT share the connection other services might
// use for regular Redis commands.
@Injectable()
export class LocationUpdatesService implements OnModuleInit, OnModuleDestroy {
  private readonly logger = new Logger(LocationUpdatesService.name);
  private subscriber: Redis;

  constructor(private readonly config: ConfigService) {}

  async onModuleInit() {
    const addr = this.config.getOrThrow<string>('REDIS_ADDR');
    const [host, port] = addr.split(':');
    const password = this.config.get<string>('REDIS_PASSWORD');
    this.subscriber = new Redis({ host, port: Number(port), password });

    await this.subscriber.subscribe(CHANNEL);
    this.logger.log(`Subscribed to Redis channel "${CHANNEL}"`);

    this.subscriber.on('message', (_channel: string, message: string) => {
      const ping = JSON.parse(message) as {
        driverId: string;
        lat: number;
        lng: number;
      };
      this.logger.log(
        `Location update via Redis: driverId=${ping.driverId} lat=${ping.lat} lng=${ping.lng}`,
      );
    });
  }

  async onModuleDestroy() {
    await this.subscriber?.quit();
  }
}
