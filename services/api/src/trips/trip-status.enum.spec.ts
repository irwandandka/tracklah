import { TripStatus, canTransition } from './trip-status.enum';

const S = TripStatus;

describe('canTransition', () => {
  it.each([
    [S.REQUESTED, S.ASSIGNED],
    [S.REQUESTED, S.CANCELLED],
    [S.ASSIGNED, S.IN_PROGRESS],
    [S.ASSIGNED, S.CANCELLED],
    [S.IN_PROGRESS, S.COMPLETED],
  ])('allows %s -> %s', (from, to) => {
    expect(canTransition(from, to)).toBe(true);
  });

  it.each([
    [S.REQUESTED, S.IN_PROGRESS],
    [S.REQUESTED, S.COMPLETED],
    [S.ASSIGNED, S.REQUESTED],
    [S.IN_PROGRESS, S.CANCELLED],
    [S.REQUESTED, S.REQUESTED],
  ])('rejects %s -> %s', (from, to) => {
    expect(canTransition(from, to)).toBe(false);
  });

  it.each([S.COMPLETED, S.CANCELLED])('treats %s as final', (from) => {
    for (const to of Object.values(TripStatus)) {
      expect(canTransition(from, to)).toBe(false);
    }
  });
});
