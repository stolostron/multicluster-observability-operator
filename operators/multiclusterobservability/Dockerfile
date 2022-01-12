# Copyright Contributors to the Open Cluster Management project

FROM registry.ci.openshift.org/stolostron/builder:go1.17-linux AS builder


WORKDIR /workspace
COPY go.sum go.mod ./
COPY ./operators/multiclusterobservability ./operators/multiclusterobservability
COPY ./operators/pkg ./operators/pkg

RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o bin/manager operators/multiclusterobservability/main.go

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

ENV OPERATOR=/usr/local/bin/mco-operator \
    USER_UID=1001 \
    USER_NAME=mco

RUN microdnf update -y && microdnf clean all

# install templates
COPY ./operators/multiclusterobservability/manifests /usr/local/manifests

# install the prestop script
COPY ./operators/multiclusterobservability/prestop.sh /usr/local/bin/prestop.sh

# install operator binary
COPY --from=builder /workspace/bin/manager ${OPERATOR}
USER ${USER_UID}

ENTRYPOINT ["/usr/local/bin/mco-operator"]
