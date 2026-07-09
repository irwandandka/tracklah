import {
  Column,
  CreateDateColumn,
  Entity,
  PrimaryGeneratedColumn,
  UpdateDateColumn,
} from 'typeorm';
import { TripStatus } from './trip-status.enum';

@Entity('trips')
export class Trip {
  @PrimaryGeneratedColumn('uuid')
  id: string;

  @Column()
  riderId: string;

  @Column({ type: 'varchar', nullable: true })
  driverId: string | null;

  @Column({ type: 'enum', enum: TripStatus, default: TripStatus.REQUESTED })
  status: TripStatus;

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
