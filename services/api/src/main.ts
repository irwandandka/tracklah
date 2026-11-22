import { NestFactory } from '@nestjs/core';
import { ValidationPipe } from '@nestjs/common';
import { AppModule } from './app.module';
import { httpMetricsMiddleware, startMetricsServer } from './metrics/metrics';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.use(httpMetricsMiddleware);
  app.useGlobalPipes(new ValidationPipe({ whitelist: true, transform: true }));
  await app.listen(process.env.PORT ?? 3000);
  // Prometheus scrapes this port directly; it is never routed publicly.
  startMetricsServer(Number(process.env.METRICS_PORT ?? 9464));
}
bootstrap();
