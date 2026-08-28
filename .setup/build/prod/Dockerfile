FROM golang:1.27.0-alpine AS BUILD

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN go build -o main ./cmd/server

CMD ["./main"]
