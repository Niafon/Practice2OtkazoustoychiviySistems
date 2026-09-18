FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/flatbook .

FROM alpine:3.22

RUN adduser -D -H -u 10001 appuser
USER appuser

COPY --from=build /out/flatbook /flatbook

EXPOSE 8080

ENTRYPOINT ["/flatbook"]