FROM golang:1.5.1

ENV GO15VENDOREXPERIMENT 1
RUN mkdir -p /go/src/github.com/signalfx/prometheustosignalfx

ADD . /go/src/github.com/signalfx/prometheustosignalfx

RUN go install github.com/signalfx/prometheustosignalfx/cmd/prometheustosfx
CMD /go/bin/prometheustosfx
