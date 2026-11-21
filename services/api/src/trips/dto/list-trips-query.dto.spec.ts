import { BadRequestException, ValidationPipe } from '@nestjs/common';
import { ListTripsQueryDto } from './list-trips-query.dto';
import { TripStatus } from '../trip-status.enum';

// Same options as main.ts, so this exercises what the controller sees.
const pipe = new ValidationPipe({ whitelist: true, transform: true });
const RIDER_ID = '22222222-2222-4222-8222-222222222222';

function parse(query: Record<string, string>) {
  return pipe.transform(query, {
    type: 'query',
    metatype: ListTripsQueryDto,
  }) as Promise<ListTripsQueryDto>;
}

describe('ListTripsQueryDto', () => {
  it('applies defaults', async () => {
    const q = await parse({ riderId: RIDER_ID });
    expect(q).toMatchObject({ riderId: RIDER_ID, limit: 20, offset: 0 });
    expect(q.status).toBeUndefined();
  });

  it('parses a comma-separated status filter and numbers', async () => {
    const q = await parse({
      riderId: RIDER_ID,
      status: 'completed, cancelled',
      limit: '5',
      offset: '10',
    });
    expect(q.status).toEqual([TripStatus.COMPLETED, TripStatus.CANCELLED]);
    expect(q.limit).toBe(5);
    expect(q.offset).toBe(10);
  });

  it.each([
    [{}],
    [{ riderId: 'rider-1' }],
    [{ riderId: RIDER_ID, status: 'completed,bogus' }],
    [{ riderId: RIDER_ID, limit: '0' }],
    [{ riderId: RIDER_ID, limit: '101' }],
    [{ riderId: RIDER_ID, offset: '-1' }],
  ])('rejects %j', async (query) => {
    await expect(parse(query)).rejects.toBeInstanceOf(BadRequestException);
  });
});
