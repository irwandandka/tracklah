import {
  Injectable,
  Logger,
  OnModuleDestroy,
  OnModuleInit,
} from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import * as amqp from 'amqplib';

const EXCHANGE = 'driver.commands';

@Injectable()
export class CommandsService implements OnModuleInit, OnModuleDestroy {
  private readonly logger = new Logger(CommandsService.name);
  private connection: amqp.ChannelModel;
  private channel: amqp.Channel;

  constructor(private readonly config: ConfigService) {}

  async onModuleInit() {
    const url = this.config.getOrThrow<string>('RABBITMQ_URL');
    this.connection = await amqp.connect(url);
    this.channel = await this.connection.createChannel();
    await this.channel.assertExchange(EXCHANGE, 'direct', { durable: true });
    this.logger.log(`Connected to RabbitMQ, exchange "${EXCHANGE}" ready`);
  }

  async onModuleDestroy() {
    await this.channel?.close();
    await this.connection?.close();
  }

  async publishToDriver(driverId: string, type: string): Promise<void> {
    const routingKey = `driver.${driverId}.command`;
    const queue = `driver-${driverId}-commands`;

    await this.channel.assertQueue(queue, { durable: true });
    await this.channel.bindQueue(queue, EXCHANGE, routingKey);

    const payload = { type, issuedAt: new Date().toISOString() };
    this.channel.publish(
      EXCHANGE,
      routingKey,
      Buffer.from(JSON.stringify(payload)),
      { persistent: true },
    );

    this.logger.log(`Published "${type}" command to driver ${driverId}`);
  }
}
