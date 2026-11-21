import {
  BadRequestException,
  ConflictException,
  Injectable,
  Logger,
  NotFoundException,
} from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { Interval } from '@nestjs/schedule';
import { InjectRepository } from '@nestjs/typeorm';
import { In, QueryFailedError, Repository } from 'typeorm';
import { Trip } from './trip.entity';
import { CreateTripDto } from './dto/create-trip.dto';
import { UpdateTripStatusDto } from './dto/update-trip-status.dto';
import { ListTripsQueryDto } from './dto/list-trips-query.dto';
import { TripEventsService } from './trip-events.service';
import { CancelReason, TripStatus, canTransition } from './trip-status.enum';

const ACTIVE_TRIP_CONSTRAINT = 'UQ_trips_rider_active';
const PG_UNIQUE_VIOLATION = '23505';
const DEFAULT_REQUEST_TIMEOUT_MINUTES = 30;
const EXPIRY_SWEEP_MS = 60_000;

export interface TripPage {
  data: Trip[];
  limit: number;
  offset: number;
  // null when there are no more pages.
  nextOffset: number | null;
}

@Injectable()
export class TripsService {
  private readonly logger = new Logger(TripsService.name);
  private readonly requestTimeoutMinutes: number;

  constructor(
    @InjectRepository(Trip)
    private readonly tripsRepository: Repository<Trip>,
    private readonly tripEvents: TripEventsService,
    config: ConfigService,
  ) {
    this.requestTimeoutMinutes = Number(
      config.get('TRIP_REQUEST_TIMEOUT_MINUTES') ??
        DEFAULT_REQUEST_TIMEOUT_MINUTES,
    );
    if (
      !Number.isInteger(this.requestTimeoutMinutes) ||
      this.requestTimeoutMinutes <= 0
    ) {
      throw new Error(
        'TRIP_REQUEST_TIMEOUT_MINUTES must be a positive whole number',
      );
    }
  }

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
    // The mobile app is the only client that cancels through the API, so
    // a cancel here is the rider's. Timeouts go through expireStaleRequests.
    const result = await this.tripsRepository.update(
      { id, status: from },
      {
        status: to,
        ...(dto.driverId ? { driverId: dto.driverId } : {}),
        ...(to === TripStatus.CANCELLED
          ? { cancelReason: CancelReason.RIDER_CANCELLED }
          : {}),
      },
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

  // A trip nobody picks up within the timeout is cancelled as
  // "no_driver_found", so the rider isn't stuck waiting (and blocked from
  // requesting again) forever. One conditional UPDATE does the check and
  // the change atomically, so it's safe to run on several API replicas
  // and can't race a driver's assign: whichever lands first wins, and the
  // other side gets 0 rows / 409.
  @Interval(EXPIRY_SWEEP_MS)
  async expireStaleRequests(): Promise<number> {
    let result: { raw: unknown };
    try {
      result = await this.tripsRepository
        .createQueryBuilder()
        .update(Trip)
        .set({
          status: TripStatus.CANCELLED,
          cancelReason: CancelReason.NO_DRIVER_FOUND,
        })
        .where('status = :status', { status: TripStatus.REQUESTED })
        .andWhere('"createdAt" < now() - make_interval(mins => :minutes)', {
          minutes: this.requestTimeoutMinutes,
        })
        .returning('*')
        .execute();
    } catch (err) {
      // A failed sweep (e.g. Postgres briefly down) just retries next tick.
      this.logger.error(`Trip expiry sweep failed: ${String(err)}`);
      return 0;
    }

    const expired = (result.raw ?? []) as Trip[];
    for (const trip of expired) {
      this.tripEvents.publish(trip);
    }
    if (expired.length > 0) {
      this.logger.log(
        `Cancelled ${expired.length} trip(s) with no driver after ${this.requestTimeoutMinutes} min`,
      );
    }
    return expired.length;
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
