# Docker Hub Exporter for Prometheus

This is a simple server that scrapes Docker Hub rate limit stats and exports them via HTTP for
Prometheus consumption.

## Getting Started

To run it:

```bash
./dockerhub_exporter [flags]
```

Help on flags:

```bash
./dockerhub_exporter -help
```

For more information check the [source code documentation][gdocs].

[gdocs]: http://godoc.org/github.com/jabley/dockerhub_exporter

## Usage

By default, it will try to use anonymous access to report on rate limits.

If you want to use an authenticated account, you can pass in your username and password using:

```bash
dockerhub_exporter  -user=<user_name> -pass=<pass_phrase>
```

### Docker

[![Docker Repository on Quay](https://quay.io/repository/jabley/dockerhub_exporter/status)][quay]

To run the Docker Hub exporter as a Docker container, run:

```bash
docker run -p 9090:9090 quay.io/jabley/dockerhub_exporter:v0.9.0
```

## Development

[![Go Report Card](https://goreportcard.com/badge/github.com/jabley/dockerhub_exporter)][goreportcard]
[![Maintainability](https://api.codeclimate.com/v1/badges/b24b9cae6fa76ce9a960/maintainability)][codeclimate]

[goreportcard]: https://goreportcard.com/report/github.com/jabley/dockerhub_exporter
[codeclimate]: https://codeclimate.com/github/jabley/dockerhub_exporter/maintainability

### Building

```bash
go build
```

### Testing

![Build Status](https://github.com/jabley/dockerhub_exporter/workflows/CICD/badge.svg)
[![Test Coverage](https://api.codeclimate.com/v1/badges/b24b9cae6fa76ce9a960/test_coverage)](https://codeclimate.com/github/jabley/dockerhub_exporter/test_coverage)

```bash
go test ./...
```

The test coverage number is interesting. Since this is (for now) a small service, it flags that all
the `func main()` bit which parses command line args isn't tested. But if you look at the report,
all of the service logic has good coverage.

## License

MIT, see [LICENSE](https://github.com/jabley/dockerhub_exporter/blob/master/LICENSE).
