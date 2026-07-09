import { Module } from '@nestjs/common';
import { TypeOrmModule } from '@nestjs/typeorm';
import { RabbitmqModule } from '../rabbitmq/rabbitmq.module';
import { Trip } from './trip.entity';
import { TripsService } from './trips.service';
import { TripsController } from './trips.controller';
import { TripEventsService } from './trip-events.service';

@Module({
  imports: [TypeOrmModule.forFeature([Trip]), RabbitmqModule],
  controllers: [TripsController],
  providers: [TripsService, TripEventsService],
})
export class TripsModule {}
