# Copyright Contributors to the Open Cluster Management project

FROM registry.ci.openshift.org/open-cluster-management/builder:go1.16-linux AS builder

ENV GOFLAGS="-mod=vendor"
COPY . /go/src/github.com/open-cluster-management/metrics-collector
RUN cd /go/src/github.com/open-cluster-management/metrics-collector && \
    go mod vendor && go mod tidy && go mod verify && \
    go build ./cmd/metrics-collector

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ARG VCS_REF
ARG VCS_URL
ARG IMAGE_NAME
ARG IMAGE_DESCRIPTION
ARG IMAGE_DISPLAY_NAME
ARG IMAGE_NAME_ARCH
ARG IMAGE_MAINTAINER
ARG IMAGE_VENDOR
ARG IMAGE_VERSION
ARG IMAGE_RELEASE
ARG IMAGE_SUMMARY
ARG IMAGE_OPENSHIFT_TAGS

LABEL org.label-schema.vendor="Red Hat" \
      org.label-schema.name="$IMAGE_NAME_ARCH" \
      org.label-schema.description="$IMAGE_DESCRIPTION" \
      org.label-schema.vcs-ref=$VCS_REF \
      org.label-schema.vcs-url=$VCS_URL \
      org.label-schema.license="Red Hat Advanced Cluster Management for Kubernetes EULA" \
      org.label-schema.schema-version="1.0" \
      name="$IMAGE_NAME" \
      maintainer="$IMAGE_MAINTAINER" \
      vendor="$IMAGE_VENDOR" \
      version="$IMAGE_VERSION" \
      release="$IMAGE_RELEASE" \
      description="$IMAGE_DESCRIPTION" \
      summary="$IMAGE_SUMMARY" \
      io.k8s.display-name="$IMAGE_DISPLAY_NAME" \
      io.k8s.description="$IMAGE_DESCRIPTION" \
      io.openshift.tags="$IMAGE_OPENSHIFT_TAGS"

RUN microdnf update &&\
    microdnf install ca-certificates vi --nodocs &&\
    mkdir /licenses &&\
    microdnf clean all

COPY --from=builder /go/src/github.com/open-cluster-management/metrics-collector/metrics-collector /usr/bin/

RUN cp /usr/bin/metrics-collector /usr/bin/telemeter-client

# standalone required parameters
ENV FROM_CA_FILE="/from/service-ca.crt"
ENV INTERVAL="60s"
ENV MATCH_FILE="/metrics/match-file"
ENV LIMIT_BYTES=1073741824

CMD ["/bin/bash", "-c", "/usr/bin/metrics-collector --from ${FROM} --from-ca-file ${FROM_CA_FILE} --from-token ${FROM_TOKEN} --to-upload ${TO_UPLOAD} --id ${TENANT_ID} --label cluster=${CLUSTER_NAME} --label clusterID=${CLUSTER_ID} --match-file ${MATCH_FILE} --interval ${INTERVAL} --limit-bytes=${LIMIT_BYTES}"]
