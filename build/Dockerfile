FROM registry.access.redhat.com/ubi8/ubi-minimal:latest

ENV OPERATOR=/usr/local/bin/multicluster-monitoring-operator \
    USER_UID=1001 \
    USER_NAME=multicluster-monitoring-operator

# install templates
COPY manifests /usr/local/manifests

# install operator binary
COPY build/_output/bin/multicluster-monitoring-operator ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
