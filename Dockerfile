# ---------------------------- build ----------------------------
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/server ./cmd/server

# --------------------------- runtime ---------------------------
FROM alpine:3

COPY --from=build /bin/server /bin/server

RUN adduser -D -u 10001 app
USER app

EXPOSE 8080

ENTRYPOINT ["/bin/server"]
