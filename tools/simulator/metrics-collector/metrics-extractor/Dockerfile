FROM registry.ci.openshift.org/stolostron/builder:go1.20-linux AS builder

RUN GOBIN=/usr/local/bin go install github.com/brancz/gojsontoyaml@latest


FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
RUN mkdir /metrics-extractor
RUN mkdir /ocp-tools
RUN microdnf install wget -y \
    && microdnf clean all
RUN microdnf install tar gzip jq bc -y\
    && microdnf clean all


RUN wget https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable-4.13/openshift-client-linux.tar.gz -P /ocp-tools
WORKDIR /ocp-tools
RUN chmod 777 /ocp-tools
RUN tar xvf openshift-client-linux.tar.gz oc kubectl
RUN rm openshift-client-linux.tar.gz
RUN cp oc /usr/local/bin
RUN cp kubectl /usr/local/bin

COPY --from=builder /usr/local/bin/gojsontoyaml /usr/local/bin/

WORKDIR /metrics-extractor

COPY ./extract-metrics-data.sh /metrics-extractor/
RUN chmod 777 /metrics-extractor


CMD [ "/bin/bash", "/metrics-extractor/extract-metrics-data.sh" ]
