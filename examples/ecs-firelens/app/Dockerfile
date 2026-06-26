FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine3.23 AS build

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/app .

FROM alpine:3.23.3

RUN adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=build /out/app /app/app

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/app"]
