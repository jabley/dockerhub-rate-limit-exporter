FROM golang:alpine as builder
LABEL maintainer="James Abley <james.abley@gmail.com>"

RUN apk add --no-cache git ca-certificates && update-ca-certificates
RUN adduser \
	--disabled-password \
	--gecos "" \
	--home "/nonexistent" \
	--no-create-home \
	--shell "/sbin/nologin" \
	--uid 10001 \
	scratchuser

WORKDIR /src/
COPY go.mod go.sum main.go ./
RUN go mod download && go mod verify
RUN CGO_ENABLED=0 go build -o dockerhub_exporter

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder ./src/dockerhub_exporter /bin/dockerhub_exporter

USER scratchuser:scratchuser
EXPOSE     9090
ENTRYPOINT ["/bin/dockerhub_exporter"]
