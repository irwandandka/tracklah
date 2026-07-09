import { Module } from '@nestjs/common';
import { RabbitmqModule } from '../rabbitmq/rabbitmq.module';
import { CommandsService } from './commands.service';
import { CommandsController } from './commands.controller';

@Module({
  imports: [RabbitmqModule],
  controllers: [CommandsController],
  providers: [CommandsService],
})
export class CommandsModule {}
