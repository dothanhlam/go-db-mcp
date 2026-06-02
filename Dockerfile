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

# Install runtime libraries for GTK and sqlite3, plus xvfb and dbus-x11 for headless systray
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    libgtk-3-0 \
    libayatana-appindicator3-1 \
    ca-certificates \
    xvfb \
    dbus-x11 \
    xauth \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /mcp-server /app/mcp-server

EXPOSE 6969
ENV PORT=6969

ENTRYPOINT ["xvfb-run", "-a", "dbus-run-session", "/app/mcp-server"]
