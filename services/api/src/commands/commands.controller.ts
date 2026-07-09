import { Body, Controller, Param, Post } from '@nestjs/common';
import { CommandsService } from './commands.service';
import { CreateCommandDto } from './dto/create-command.dto';

@Controller('drivers/:driverId/commands')
export class CommandsController {
  constructor(private readonly commandsService: CommandsService) {}

  @Post()
  async create(
    @Param('driverId') driverId: string,
    @Body() dto: CreateCommandDto,
  ) {
    await this.commandsService.publishToDriver(driverId, dto.type);
    return { status: 'queued', driverId, type: dto.type };
  }
}
