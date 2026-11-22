import { MigrationInterface, QueryRunner } from 'typeorm';

export class InitialSchema1795275518308 implements MigrationInterface {
  name = 'InitialSchema1795275518308';

  public async up(queryRunner: QueryRunner): Promise<void> {
    await queryRunner.query(
      `CREATE TYPE "public"."trips_status_enum" AS ENUM('requested', 'assigned', 'in_progress', 'completed', 'cancelled')`,
    );
    await queryRunner.query(
      `CREATE TYPE "public"."trips_cancelreason_enum" AS ENUM('rider_cancelled', 'no_driver_found')`,
    );
    await queryRunner.query(
      `CREATE TABLE "trips" ("id" uuid NOT NULL DEFAULT gen_random_uuid(), "riderId" character varying NOT NULL, "driverId" character varying, "status" "public"."trips_status_enum" NOT NULL DEFAULT 'requested', "cancelReason" "public"."trips_cancelreason_enum", "originLat" double precision NOT NULL, "originLng" double precision NOT NULL, "destinationLat" double precision NOT NULL, "destinationLng" double precision NOT NULL, "createdAt" TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(), "updatedAt" TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(), CONSTRAINT "PK_f71c231dee9c05a9522f9e840f5" PRIMARY KEY ("id"))`,
    );
    await queryRunner.query(
      `CREATE UNIQUE INDEX "UQ_trips_rider_active" ON "trips" ("riderId") WHERE "status" IN ('requested', 'assigned', 'in_progress')`,
    );
    await queryRunner.query(
      `CREATE INDEX "IDX_trips_rider_created" ON "trips" ("riderId", "createdAt") `,
    );
  }

  public async down(queryRunner: QueryRunner): Promise<void> {
    await queryRunner.query(`DROP INDEX "public"."IDX_trips_rider_created"`);
    await queryRunner.query(`DROP INDEX "public"."UQ_trips_rider_active"`);
    await queryRunner.query(`DROP TABLE "trips"`);
    await queryRunner.query(`DROP TYPE "public"."trips_cancelreason_enum"`);
    await queryRunner.query(`DROP TYPE "public"."trips_status_enum"`);
  }
}
