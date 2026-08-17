# --- build stage ---
FROM golang:1.22-alpine AS build

WORKDIR /src

# Copy just the module files first so go mod download gets cached
# separately from source changes.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 gets us a static binary with no libc dependency, which
# distroless needs. -ldflags="-s -w" strips debug symbols to keep it small.
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/server ./cmd/server

# --- final stage ---
# distroless/static has no shell, no package manager, nothing beyond the
# binary and its runtime deps (none here, since CGO is off). Smaller
# attack surface - no shell to pop, nothing extra to patch - which felt
# like the right call for a security-focused company's image.
FROM gcr.io/distroless/static-debian12

# ships with a built-in nonroot user (uid/gid 65532), so we're not
# running as root by default.
USER nonroot:nonroot

COPY --from=build /out/server /server

EXPOSE 8080

ENTRYPOINT ["/server"]
