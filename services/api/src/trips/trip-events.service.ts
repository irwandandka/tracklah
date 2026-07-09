import { Injectable, Logger, OnModuleInit } from '@nestjs/common';
import { RabbitmqService } from '../rabbitmq/rabbitmq.service';
import { Trip } from './trip.entity';

const EXCHANGE = 'trip.lifecycle';

@Injectable()
export class TripEventsService implements OnModuleInit {
  private readonly logger = new Logger(TripEventsService.name);

  constructor(private readonly rabbitmq: RabbitmqService) {}

  async onModuleInit() {
    await this.rabbitmq
      .getChannel()
      .assertExchange(EXCHANGE, 'fanout', { durable: true });
    this.logger.log(`Exchange "${EXCHANGE}" ready`);
  }

  publish(trip: Trip): void {
    const payload = {
      tripId: trip.id,
      status: trip.status,
      driverId: trip.driverId,
      occurredAt: new Date().toISOString(),
    };

    // Fanout ignores the routing key entirely - every queue bound to this
    // exchange gets its own copy of the message, regardless of what's
    // passed here. '' is the conventional placeholder for "not used".
    this.rabbitmq
      .getChannel()
      .publish(EXCHANGE, '', Buffer.from(JSON.stringify(payload)), {
        persistent: true,
      });

    this.logger.log(`Broadcast trip ${trip.id} status=${trip.status}`);
  }
}
