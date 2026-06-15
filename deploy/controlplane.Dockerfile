FROM golang:1.26.4-alpine AS build
ENV GOWORK=off
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/forge-controlplane ./cmd/forge-controlplane

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/forge-controlplane /forge-controlplane
EXPOSE 8080 9090
ENTRYPOINT ["/forge-controlplane"]
