import { BadRequestException, ConflictException } from '@nestjs/common';
import { QueryFailedError, Repository } from 'typeorm';
import { TripsService } from './trips.service';
import { TripEventsService } from './trip-events.service';
import { Trip } from './trip.entity';
import { TripStatus } from './trip-status.enum';

const TRIP_ID = '11111111-1111-4111-8111-111111111111';
const RIDER_ID = '22222222-2222-4222-8222-222222222222';

function makeTrip(overrides: Partial<Trip> = {}): Trip {
  return {
    id: TRIP_ID,
    riderId: RIDER_ID,
    driverId: null,
    status: TripStatus.REQUESTED,
    originLat: -6.2,
    originLng: 106.8,
    destinationLat: -6.21,
    destinationLng: 106.81,
    createdAt: new Date(),
    updatedAt: new Date(),
    ...overrides,
  };
}

describe('TripsService', () => {
  let repo: jest.Mocked<
    Pick<Repository<Trip>, 'create' | 'save' | 'find' | 'findOneBy' | 'update'>
  >;
  let events: { publish: jest.Mock };
  let service: TripsService;

  beforeEach(() => {
    repo = {
      create: jest.fn((dto) => dto as Trip),
      save: jest.fn(),
      find: jest.fn(),
      findOneBy: jest.fn(),
      update: jest.fn(),
    };
    events = { publish: jest.fn() };
    service = new TripsService(
      repo as unknown as Repository<Trip>,
      events as unknown as TripEventsService,
    );
  });

  describe('create', () => {
    const dto = {
      riderId: RIDER_ID,
      originLat: -6.2,
      originLng: 106.8,
      destinationLat: -6.21,
      destinationLng: 106.81,
    };

    it('saves and publishes the new trip', async () => {
      const saved = makeTrip();
      repo.save.mockResolvedValue(saved);
      await expect(service.create(dto)).resolves.toBe(saved);
      expect(events.publish).toHaveBeenCalledWith(saved);
    });

    it('maps the active-trip unique violation to 409', async () => {
      const err = new QueryFailedError('INSERT', [], {
        code: '23505',
        constraint: 'UQ_trips_rider_active',
      } as unknown as Error);
      repo.save.mockRejectedValue(err);
      await expect(service.create(dto)).rejects.toBeInstanceOf(
        ConflictException,
      );
      expect(events.publish).not.toHaveBeenCalled();
    });

    it('rethrows other database errors', async () => {
      repo.save.mockRejectedValue(new Error('connection lost'));
      await expect(service.create(dto)).rejects.toThrow('connection lost');
    });
  });

  describe('updateStatus', () => {
    it('assigns a driver with a compare-and-set on the old status', async () => {
      const assigned = makeTrip({
        status: TripStatus.ASSIGNED,
        driverId: 'driver-1',
      });
      repo.findOneBy
        .mockResolvedValueOnce(makeTrip())
        .mockResolvedValueOnce(assigned);
      repo.update.mockResolvedValue({
        affected: 1,
        raw: [],
        generatedMaps: [],
      });

      await expect(
        service.updateStatus(TRIP_ID, {
          status: TripStatus.ASSIGNED,
          driverId: 'driver-1',
        }),
      ).resolves.toBe(assigned);
      expect(repo.update).toHaveBeenCalledWith(
        { id: TRIP_ID, status: TripStatus.REQUESTED },
        { status: TripStatus.ASSIGNED, driverId: 'driver-1' },
      );
      expect(events.publish).toHaveBeenCalledWith(assigned);
    });

    it('rejects an invalid transition with 409', async () => {
      repo.findOneBy.mockResolvedValue(
        makeTrip({ status: TripStatus.COMPLETED }),
      );
      await expect(
        service.updateStatus(TRIP_ID, { status: TripStatus.CANCELLED }),
      ).rejects.toBeInstanceOf(ConflictException);
      expect(repo.update).not.toHaveBeenCalled();
    });

    it('requires driverId when assigning', async () => {
      repo.findOneBy.mockResolvedValue(makeTrip());
      await expect(
        service.updateStatus(TRIP_ID, { status: TripStatus.ASSIGNED }),
      ).rejects.toBeInstanceOf(BadRequestException);
    });

    it('rejects driverId on non-assign transitions', async () => {
      repo.findOneBy.mockResolvedValue(
        makeTrip({ status: TripStatus.ASSIGNED, driverId: 'driver-1' }),
      );
      await expect(
        service.updateStatus(TRIP_ID, {
          status: TripStatus.IN_PROGRESS,
          driverId: 'driver-2',
        }),
      ).rejects.toBeInstanceOf(BadRequestException);
    });

    it('returns 409 when the trip changed concurrently', async () => {
      repo.findOneBy.mockResolvedValue(makeTrip());
      repo.update.mockResolvedValue({
        affected: 0,
        raw: [],
        generatedMaps: [],
      });
      await expect(
        service.updateStatus(TRIP_ID, { status: TripStatus.CANCELLED }),
      ).rejects.toBeInstanceOf(ConflictException);
      expect(events.publish).not.toHaveBeenCalled();
    });
  });

  describe('findForRider', () => {
    it('returns nextOffset when there is another page', async () => {
      repo.find.mockResolvedValue([makeTrip(), makeTrip(), makeTrip()]);
      const page = await service.findForRider({
        riderId: RIDER_ID,
        limit: 2,
        offset: 0,
      });
      expect(page.data).toHaveLength(2);
      expect(page.nextOffset).toBe(2);
      expect(repo.find).toHaveBeenCalledWith(
        expect.objectContaining({
          where: { riderId: RIDER_ID },
          order: { createdAt: 'DESC', id: 'DESC' },
          take: 3,
          skip: 0,
        }),
      );
    });

    it('returns null nextOffset on the last page', async () => {
      repo.find.mockResolvedValue([makeTrip()]);
      const page = await service.findForRider({
        riderId: RIDER_ID,
        status: [TripStatus.COMPLETED],
        limit: 20,
        offset: 20,
      });
      expect(page.data).toHaveLength(1);
      expect(page.nextOffset).toBeNull();
    });
  });
});
