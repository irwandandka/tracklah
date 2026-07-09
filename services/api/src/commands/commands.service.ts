import { Injectable, Logger, OnModuleInit } from '@nestjs/common';
import { RabbitmqService } from '../rabbitmq/rabbitmq.service';

const EXCHANGE = 'driver.commands';

@Injectable()
export class CommandsService implements OnModuleInit {
  private readonly logger = new Logger(CommandsService.name);

  constructor(private readonly rabbitmq: RabbitmqService) {}

  async onModuleInit() {
    await this.rabbitmq
      .getChannel()
      .assertExchange(EXCHANGE, 'direct', { durable: true });
    this.logger.log(`Exchange "${EXCHANGE}" ready`);
  }

  async publishToDriver(driverId: string, type: string): Promise<void> {
    const channel = this.rabbitmq.getChannel();
    const routingKey = `driver.${driverId}.command`;
    const queue = `driver-${driverId}-commands`;

    await channel.assertQueue(queue, { durable: true });
    await channel.bindQueue(queue, EXCHANGE, routingKey);

    const payload = { type, issuedAt: new Date().toISOString() };
    channel.publish(
      EXCHANGE,
      routingKey,
      Buffer.from(JSON.stringify(payload)),
      { persistent: true },
    );

    this.logger.log(`Published "${type}" command to driver ${driverId}`);
  }
}
