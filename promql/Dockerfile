FROM golang:1.18 as build-env

WORKDIR /go/src/promql
COPY . /go/src/promql

ENV CGO_ENABLED 0

RUN go build ./cmd/promql-compliance-tester

FROM quay.io/prometheus/busybox
COPY --from=build-env /go/src/promql/promql-compliance-tester /
COPY --from=build-env /go/src/promql/promql-test-queries.yml /

ENTRYPOINT ["/promql-compliance-tester"]
