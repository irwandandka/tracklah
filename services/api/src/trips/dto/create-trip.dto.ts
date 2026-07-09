import { IsLatitude, IsLongitude, IsString } from 'class-validator';

export class CreateTripDto {
  @IsString()
  riderId: string;

  @IsLatitude()
  originLat: number;

  @IsLongitude()
  originLng: number;

  @IsLatitude()
  destinationLat: number;

  @IsLongitude()
  destinationLng: number;
}
