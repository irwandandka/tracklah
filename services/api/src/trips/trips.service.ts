import { Injectable, NotFoundException } from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { Repository } from 'typeorm';
import { Trip } from './trip.entity';
import { CreateTripDto } from './dto/create-trip.dto';
import { UpdateTripStatusDto } from './dto/update-trip-status.dto';

@Injectable()
export class TripsService {
  constructor(
    @InjectRepository(Trip)
    private readonly tripsRepository: Repository<Trip>,
  ) {}

  create(dto: CreateTripDto): Promise<Trip> {
    const trip = this.tripsRepository.create(dto);
    return this.tripsRepository.save(trip);
  }

  findAll(): Promise<Trip[]> {
    return this.tripsRepository.find();
  }

  async findOne(id: string): Promise<Trip> {
    const trip = await this.tripsRepository.findOneBy({ id });
    if (!trip) {
      throw new NotFoundException(`Trip ${id} not found`);
    }
    return trip;
  }

  async updateStatus(id: string, dto: UpdateTripStatusDto): Promise<Trip> {
    const trip = await this.findOne(id);
    trip.status = dto.status;
    if (dto.driverId) {
      trip.driverId = dto.driverId;
    }
    return this.tripsRepository.save(trip);
  }
}
