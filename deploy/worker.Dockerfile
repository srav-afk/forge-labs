FROM golang:1.26.4-alpine AS build
ENV GOWORK=off
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/forge-worker ./cmd/forge-worker

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/forge-worker /forge-worker
EXPOSE 9091
ENTRYPOINT ["/forge-worker"]
