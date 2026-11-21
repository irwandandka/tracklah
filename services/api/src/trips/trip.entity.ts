import {
  Column,
  CreateDateColumn,
  Entity,
  Index,
  PrimaryGeneratedColumn,
  UpdateDateColumn,
} from 'typeorm';
import { CancelReason, TripStatus } from './trip-status.enum';

@Entity('trips')
// Backs GET /trips?riderId=... ordered by createdAt DESC.
@Index('IDX_trips_rider_created', ['riderId', 'createdAt'])
// Enforces "one active trip per rider" in the database itself, so two
// concurrent POST /trips can't both slip past an application-level check.
// Keep the status list in sync with ACTIVE_STATUSES.
@Index('UQ_trips_rider_active', ['riderId'], {
  unique: true,
  where: `"status" IN ('requested', 'assigned', 'in_progress')`,
})
export class Trip {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  // varchar, not uuid: changing the column type under synchronize: true
  // would drop and re-add it. The UUID format is enforced in the DTO.
  @Column()
  riderId: string;

  @Column({ type: 'varchar', nullable: true })
  driverId: string | null;

  @Column({ type: 'enum', enum: TripStatus, default: TripStatus.REQUESTED })
  status: TripStatus;

  // Only set when status is "cancelled".
  @Column({ type: 'enum', enum: CancelReason, nullable: true })
  cancelReason: CancelReason | null;

  @Column('double precision')
  originLat: number;

  @Column('double precision')
  originLng: number;

  @Column('double precision')
  destinationLat: number;

  @Column('double precision')
  destinationLng: number;

  @CreateDateColumn()
  createdAt: Date;

  @UpdateDateColumn()
  updatedAt: Date;
}
