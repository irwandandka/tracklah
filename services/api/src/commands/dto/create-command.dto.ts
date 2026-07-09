import { IsString } from 'class-validator';

export class CreateCommandDto {
  @IsString()
  type: string;
}
