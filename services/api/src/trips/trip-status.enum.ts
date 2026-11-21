export enum TripStatus {
  REQUESTED = 'requested',
  ASSIGNED = 'assigned',
  IN_PROGRESS = 'in_progress',
  COMPLETED = 'completed',
  CANCELLED = 'cancelled',
}

// The trip state machine. Anything not listed here is rejected with 409,
// and an empty list means the status is final.
export const ALLOWED_TRANSITIONS: Record<TripStatus, readonly TripStatus[]> = {
  [TripStatus.REQUESTED]: [TripStatus.ASSIGNED, TripStatus.CANCELLED],
  [TripStatus.ASSIGNED]: [TripStatus.IN_PROGRESS, TripStatus.CANCELLED],
  [TripStatus.IN_PROGRESS]: [TripStatus.COMPLETED],
  [TripStatus.COMPLETED]: [],
  [TripStatus.CANCELLED]: [],
};

// A rider can have at most one trip in any of these at a time.
export const ACTIVE_STATUSES: readonly TripStatus[] = [
  TripStatus.REQUESTED,
  TripStatus.ASSIGNED,
  TripStatus.IN_PROGRESS,
];

export function canTransition(from: TripStatus, to: TripStatus): boolean {
  return ALLOWED_TRANSITIONS[from].includes(to);
}
