import {
  BadRequestException,
  ConflictException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import { InjectRepository } from '@nestjs/typeorm';
import { In, QueryFailedError, Repository } from 'typeorm';
import { Trip } from './trip.entity';
import { CreateTripDto } from './dto/create-trip.dto';
import { UpdateTripStatusDto } from './dto/update-trip-status.dto';
import { ListTripsQueryDto } from './dto/list-trips-query.dto';
import { TripEventsService } from './trip-events.service';
import { TripStatus, canTransition } from './trip-status.enum';

const ACTIVE_TRIP_CONSTRAINT = 'UQ_trips_rider_active';
const PG_UNIQUE_VIOLATION = '23505';

export interface TripPage {
  data: Trip[];
  limit: number;
  offset: number;
  // null when there are no more pages.
  nextOffset: number | null;
}

@Injectable()
export class TripsService {
  constructor(
    @InjectRepository(Trip)
    private readonly tripsRepository: Repository<Trip>,
    private readonly tripEvents: TripEventsService,
  ) {}

  async create(dto: CreateTripDto): Promise<Trip> {
    const trip = this.tripsRepository.create(dto);
    let saved: Trip;
    try {
      saved = await this.tripsRepository.save(trip);
    } catch (err) {
      if (isActiveTripViolation(err)) {
        throw new ConflictException(
          `Rider ${dto.riderId} already has an active trip`,
        );
      }
      throw err;
    }
    this.tripEvents.publish(saved);
    return saved;
  }

  async findForRider(query: ListTripsQueryDto): Promise<TripPage> {
    const { riderId, status, limit, offset } = query;
    // Fetch one extra row to know whether another page exists without a
    // separate COUNT(*).
    const rows = await this.tripsRepository.find({
      where: { riderId, ...(status ? { status: In(status) } : {}) },
      order: { createdAt: 'DESC', id: 'DESC' },
      take: limit + 1,
      skip: offset,
    });
    const hasMore = rows.length > limit;
    return {
      data: hasMore ? rows.slice(0, limit) : rows,
      limit,
      offset,
      nextOffset: hasMore ? offset + limit : null,
    };
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
    const from = trip.status;
    const to = dto.status;

    if (!canTransition(from, to)) {
      throw new ConflictException(
        `Cannot change trip ${id} from "${from}" to "${to}"`,
      );
    }
    if (to === TripStatus.ASSIGNED && !dto.driverId) {
      throw new BadRequestException('driverId is required when assigning');
    }
    if (to !== TripStatus.ASSIGNED && dto.driverId) {
      throw new BadRequestException('driverId can only be set when assigning');
    }

    // Compare-and-set on the status we just read: if something else
    // (e.g. the simulator and a rider cancel racing each other) changed
    // the trip in between, zero rows match and this request loses with
    // 409 instead of silently overwriting the other change.
    const result = await this.tripsRepository.update(
      { id, status: from },
      { status: to, ...(dto.driverId ? { driverId: dto.driverId } : {}) },
    );
    if (!result.affected) {
      throw new ConflictException(
        `Trip ${id} changed while updating, re-fetch and retry`,
      );
    }

    const saved = await this.findOne(id);
    this.tripEvents.publish(saved);
    return saved;
  }
}

function isActiveTripViolation(err: unknown): boolean {
  if (!(err instanceof QueryFailedError)) return false;
  const driverError = err.driverError as { code?: string; constraint?: string };
  return (
    driverError.code === PG_UNIQUE_VIOLATION &&
    driverError.constraint === ACTIVE_TRIP_CONSTRAINT
  );
}
