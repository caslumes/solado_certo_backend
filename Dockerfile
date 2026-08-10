FROM golang:1.26.5

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

RUN go install github.com/air-verse/air@latest

COPY . .

EXPOSE 3000

CMD ["air", "-c", ".air.toml"]
