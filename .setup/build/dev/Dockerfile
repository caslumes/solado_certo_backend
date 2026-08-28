FROM golang:1.27.0-alpine AS BUILD

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download \
&& go install github.com/air-verse/air@latest \
&& go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

COPY . .

EXPOSE 3000

CMD ["air", "-c", ".air.toml"]
