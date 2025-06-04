# Copyright Contributors to the Open Cluster Management project

FROM registry.ci.openshift.org/stolostron/builder:go1.23-linux AS builder

WORKDIR /workspace
COPY go.sum go.mod ./
COPY tools/simulator/alert-forward/main.go tools/simulator/alert-forward/main.go

RUN CGO_ENABLED=1 go build -a -installsuffix cgo -o bin/alert-forwarder tools/simulator/alert-forward/main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ENV MAIN_BINARY=/usr/local/bin/alert-forwarder \
    USER_UID=1001 \
    USER_NAME=alert-forwarder

# install the binary
COPY --from=builder /workspace/bin/alert-forwarder ${MAIN_BINARY} 
COPY tools/simulator/alert-forward/alerts.json /tmp/

USER ${USER_UID}

ENTRYPOINT ["/usr/local/bin/alert-forwarder"]
