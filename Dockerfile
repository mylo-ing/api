# Dockerfile for building the Fiber GORM API
FROM golang:1.24-alpine AS build

RUN apk add --no-cache postgresql-client

WORKDIR /app

COPY go.mod ./
RUN go mod download && go mod verify

COPY . .

RUN go build -v -o main .
