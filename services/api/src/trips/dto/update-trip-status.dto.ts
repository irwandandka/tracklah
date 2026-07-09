import { IsEnum, IsOptional, IsString } from 'class-validator';
import { TripStatus } from '../trip-status.enum';

export class UpdateTripStatusDto {
  @IsEnum(TripStatus)
  status: TripStatus;

  @IsOptional()
  @IsString()
  driverId?: string;
}
