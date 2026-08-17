# stockticker

A small HTTP service that reports the average closing price of a single
stock symbol over a fixed number of trading days, backed by the
[Alpha Vantage](https://www.alphavantage.co/) `TIME_SERIES_DAILY` API.

The symbol and day count are fixed at startup via environment variables
rather than accepted per-request — `GET /` always returns the average for
whatever symbol/days combination the service was started with. Results are
cached in memory for a configurable TTL so we're not hammering Alpha
Vantage's (fairly aggressive) rate limits.

## Why it's built this way

**Caching, mainly to protect the API quota.** Alpha Vantage's free tier
only allows a handful of requests a day, so a generic, thread-safe
`TTLCache` (`internal/cache`) sits in front of `stockservice`, keyed by
symbol. Repeated requests within the TTL window get served from memory
instead of burning quota.

**Config gets validated up front, not discovered at request time.**
`SYMBOL` and `NDAYS` are required and checked at startup
(`cmd/server/main.go`) — if either is missing, or `NDAYS` isn't a positive
integer, the process exits immediately with a clear error instead of
limping along and failing confusingly on the first real request.

**Alpha Vantage has an annoying quirk worth calling out.** It returns HTTP
200 even when you've hit a rate limit or passed an invalid symbol — the
actual error shows up buried in the JSON body, under `"Note"`,
`"Information"`, or `"Error Message"`. `internal/alphavantage` checks for
these explicitly rather than trusting the status code.

**The business logic doesn't know about the network.** `stockservice.Service`
depends on a small `DailyPointSource` interface rather than the concrete
`alphavantage.Client`, so the averaging logic can be unit tested with an
in-memory fake and no network calls.

**Docker image is distroless and multi-stage.** Built in a
`golang:1.22-alpine` stage, shipped on `gcr.io/distroless/static-debian12`,
which has no shell or package manager at all. Smaller attack surface —
nothing to pop a shell in, nothing extra to patch — which felt like the
right call for a security-focused deployment.

**Shutdown is graceful, not abrupt.** The server listens for
`SIGINT`/`SIGTERM` and drains in-flight requests via `http.Server.Shutdown`
(10s timeout) instead of just dying mid-response.

## Running it locally

```sh
export SYMBOL=MSFT
export NDAYS=5
go run ./cmd/server
```

Then, in another terminal:

```sh
curl http://localhost:8080/
curl http://localhost:8080/healthz
```

## Running the tests

```sh
go test ./... -v
```

## Building and running with Docker

The easiest path is to just pull the image — a build + push to
[GHCR](https://ghcr.io/catherinesanft/ping-identity-tech-task) happens
automatically via GitHub Actions
(`.github/workflows/docker-publish.yml`) on every push to `main`:

```sh
docker pull ghcr.io/catherinesanft/ping-identity-tech-task:latest

docker run --rm -p 8080:8080 \
  -e SYMBOL=MSFT \
  -e NDAYS=5 \
  ghcr.io/catherinesanft/ping-identity-tech-task:latest
```

One catch: that image is `linux/amd64` only. On Apple Silicon, add
`--platform linux/amd64` to both commands above, or just build it
yourself instead:

```sh
docker build -t stockticker:latest .

docker run --rm -p 8080:8080 \
  -e SYMBOL=MSFT \
  -e NDAYS=5 \
  stockticker:latest
```

Either way, you can hit it the same way as running it locally:

```sh
curl http://localhost:8080/
```

## Environment variables

| Variable    | Required | Default            | Description                                                          |
|-------------|----------|---------------------|------------------------------------------------------------------------|
| `SYMBOL`    | yes      | —                    | Stock ticker symbol to report on (e.g. `MSFT`).                       |
| `NDAYS`     | yes      | —                    | Number of most recent trading days to average. Must be a positive integer. |
| `APIKEY`    | no       | `C227WD9W3LUVKVV9`   | Alpha Vantage API key. Defaults to the sample key from the exercise spec. |
| `PORT`      | no       | `8080`               | Port the HTTP server listens on.                                      |
| `CACHE_TTL` | no       | `5m`                 | How long a computed result is cached per symbol, as a Go duration string (e.g. `30s`, `5m`, `1h`). |

## Known limitations

- No `HEALTHCHECK` in the Docker image. Distroless has no shell, so there's
  no way to run the usual `CMD curl ...` style healthcheck inside the
  container. Liveness needs to be checked externally instead — e.g. an
  orchestrator hitting `/healthz` over HTTP.
- This reports on one fixed symbol/day-count combination per running
  instance. It's not a general-purpose multi-symbol API.
- The in-memory cache is per-process, not shared across replicas — a
  multi-instance deployment behind a load balancer will re-fetch from
  Alpha Vantage once per replica instead of sharing a cached result. In
  practice that means a multi-replica deployment would either need to
  stay pinned to a single replica, or accept duplicate upstream calls per
  replica until a shared cache (e.g. Redis) replaces the in-memory one.
  That's why the Kubernetes Deployment in `k8s/` is pinned to one replica.
