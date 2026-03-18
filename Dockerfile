FROM golang:1.24-alpine AS builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o ssh-portfolio .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/ssh-portfolio .

RUN mkdir -p .ssh

EXPOSE 106

CMD ["./ssh-portfolio"]
