FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o server ./cmd/ordersystem/main.go ./cmd/ordersystem/wire_gen.go

FROM scratch

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/.env .

EXPOSE 8000 50051 8090

CMD ["./server"]
