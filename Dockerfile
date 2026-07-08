FROM golang:bookworm AS builder

WORKDIR /app

# Install dependencies for CGO (sqlite3) and systray (gtk3, appindicator)
RUN apt-get update && \
    apt-get install -y gcc pkg-config libgtk-3-dev libayatana-appindicator3-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /mcp-server main.go

FROM debian:bookworm-slim

WORKDIR /app

# Runtime libraries. The binary links GTK/appindicator (systray is compiled in),
# so the shared objects must be present even though the tray is not started in
# headless mode.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    libgtk-3-0 \
    libayatana-appindicator3-1 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /mcp-server /app/mcp-server

EXPOSE 6969
ENV PORT=6969
# Run without the system tray: no display, D-Bus, or Xvfb needed. Headless mode
# also binds 0.0.0.0 by default so the published port is reachable.
ENV HEADLESS=1

ENTRYPOINT ["/app/mcp-server"]
