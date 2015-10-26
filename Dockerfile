FROM golang:latest

RUN mkdir /go/src/prometheustosignalfx

RUN go get github.com/codegangsta/cli github.com/signalfx/golib/datapoint github.com/signalfx/metricproxy/protocol/signalfx github.com/gogo/protobuf/proto github.com/prometheus/client_golang/prometheus github.com/prometheus/client_model/go

ADD ./prometheustosignalfx /go/src/prometheustosignalfx

ADD run.sh /go/run.sh
RUN chmod +x /go/run.sh
RUN /go/run.sh
#RUN ls -la

#RUN export GO15VENDOREXPERIMENT=1
#RUN run.sh
#RUN go install -v -x prometheustosignalfx/scrapper
CMD /go/bin/scrapper