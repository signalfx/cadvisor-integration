FROM golang:latest

RUN mkdir -p /go/src/github.com/signalfx/prometheustosignalfx

ADD . /go/src/github.com/signalfx/prometheustosignalfx

ADD go install github.com/signalfx/prometheustosignalfx/cmd/prometheustosfx
CMD /go/bin/prometheustosfx
