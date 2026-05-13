FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/fulltankgarage-api ./cmd/api

FROM alpine:3.22

WORKDIR /app
ENV UPLOAD_DIR=/app/uploads
COPY --from=build /out/fulltankgarage-api /app/fulltankgarage-api
COPY --from=build /src/assets /app/assets
COPY .env.example /app/.env.example
RUN mkdir -p /app/uploads/storefronts /app/uploads/receipts

EXPOSE 8080
CMD ["/app/fulltankgarage-api"]
