FROM golang:1.5.1

ENV GO15VENDOREXPERIMENT 1
RUN mkdir -p /go/src/github.com/signalfx/cadvisor-integration

ADD . /go/src/github.com/signalfx/cadvisor-integration

RUN go install -ldflags "-X poller.ToolVersion=`cd /go/src/github.com/signalfx/cadvisor-integration;git log --oneline -n 1 | awk '{print $1}'`" github.com/signalfx/cadvisor-integration/cmd/cadvisortosfx
CMD /go/bin/cadvisortosfx
