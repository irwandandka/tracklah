import { IsEnum, IsNotEmpty, IsOptional, IsString } from 'class-validator';
import { TripStatus } from '../trip-status.enum';

export class UpdateTripStatusDto {
  @IsEnum(TripStatus)
  status: TripStatus;

  // Required when status is "assigned", rejected otherwise (checked in
  // TripsService, since it depends on the target status).
  @IsOptional()
  @IsString()
  @IsNotEmpty()
  driverId?: string;
}
