# Multi-stage image for Coolify / Compose (Phase A PoC).
# Binds all interfaces when PORT is set; data lives on a volume at /data.

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/school-nanny .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/school-nanny /school-nanny
ENV PORT=8080
EXPOSE 8080
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/school-nanny"]
CMD ["-data", "/data"]
