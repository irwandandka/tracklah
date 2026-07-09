import { Module } from '@nestjs/common';
import { LocationUpdatesService } from './location-updates.service';

@Module({
  providers: [LocationUpdatesService],
})
export class LocationModule {}
