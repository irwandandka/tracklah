FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o trip-events-consumer .

FROM alpine:3.20 AS production
WORKDIR /app
COPY --from=build /app/trip-events-consumer .
CMD ["./trip-events-consumer"]
