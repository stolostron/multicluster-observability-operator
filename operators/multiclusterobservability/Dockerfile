# copyright contributors to the open cluster management project

from registry.ci.openshift.org/stolostron/builder:go1.23-linux as builder


workdir /workspace
copy go.sum go.mod ./
copy ./operators/multiclusterobservability ./operators/multiclusterobservability
copy ./operators/pkg ./operators/pkg

run cgo_enabled=1 go build -a -installsuffix cgo -o bin/manager operators/multiclusterobservability/main.go

from registry.access.redhat.com/ubi9/ubi-minimal:latest

arg vcs_ref
arg vcs_url
arg image_name
arg image_description
arg image_display_name
arg image_name_arch
arg image_maintainer
arg image_vendor
arg image_version
arg image_release
arg image_summary
arg image_openshift_tags

label org.label-schema.vendor="red hat" \
    org.label-schema.name="$image_name_arch" \
    org.label-schema.description="$image_description" \
    org.label-schema.vcs-ref=$vcs_ref \
    org.label-schema.vcs-url=$vcs_url \
    org.label-schema.license="red hat advanced cluster management for kubernetes eula" \
    org.label-schema.schema-version="1.0" \
    name="$image_name" \
    maintainer="$image_maintainer" \
    vendor="$image_vendor" \
    version="$image_version" \
    release="$image_release" \
    description="$image_description" \
    summary="$image_summary" \
    io.k8s.display-name="$image_display_name" \
    io.k8s.description="$image_description" \
    io.openshift.tags="$image_openshift_tags"

env operator=/usr/local/bin/mco-operator \
    user_uid=1001 \
    user_name=mco

run microdnf update -y && microdnf clean all

# install templates
copy ./operators/multiclusterobservability/manifests /usr/local/manifests

# install the prestop script
copy ./operators/multiclusterobservability/prestop.sh /usr/local/bin/prestop.sh

# install operator binary
copy --from=builder /workspace/bin/manager ${operator}
user ${user_uid}

entrypoint ["/usr/local/bin/mco-operator"]
