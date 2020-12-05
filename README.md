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
docker run -p 9101:9101 quay.io/jabley/dockerhub_exporter:v0.9.0
```

## Development

[![Go Report Card](https://goreportcard.com/badge/github.com/jabley/dockerhub_exporter)][goreportcard]
[![Code Climate](https://codeclimate.com/github/jabley/dockerhub_exporter/badges/gpa.svg)][codeclimate]

[goreportcard]: https://goreportcard.com/report/github.com/jabley/dockerhub_exporter
[codeclimate]: https://codeclimate.com/github/jabley/dockerhub_exporter

### Building

```bash
make build
```

### Testing

[![Build Status](https://travis-ci.org/jabley/dockerhub_exporter_.png?branch=master)][travisci]
[![CircleCI](https://circleci.com/gh/jabley/dockerhub_exporter/tree/master.svg?style=shield)][circleci]

```bash
make test
```

[travisci]: https://travis-ci.org/jabley/dockerhub_exporter_
[circleci]: https://circleci.com/gh/jabley/dockerhub_exporter_

## License

MIT, see [LICENSE](https://github.com/jabley/dockerhub_exporter/blob/master/LICENSE).
