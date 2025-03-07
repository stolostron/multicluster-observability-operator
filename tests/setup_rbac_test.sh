#!/usr/bin/env bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
2
create_test_users() {
  echo CREATING USER PASSWORDS SECRET
  htpasswd -c -B -b users.htpasswd admin admin
  htpasswd -B -b users.htpasswd user1 user1
  htpasswd -B -b users.htpasswd user2 user2
  oc create ns openshift-config
  oc delete secret htpass-user-test -n openshift-config &> /dev/null
  oc create secret generic htpass-user-test --from-file=htpasswd=users.htpasswd -n openshift-config
  rm -f users.htpasswd
}

create_auth_provider() {
  echo CREATING AUTH PROVIDER
  cat >oauth.yaml << EOL
apiVersion: config.openshift.io/v1
kind: OAuth
metadata:
    name: cluster
spec:
    identityProviders:
        - name: users
          mappingMethod: claim
          type: HTPasswd
          htpasswd:
              fileData:
                  name: htpass-secret
EOL
  echo oc apply -f oauth.yaml
}

create_role_bindings() {
  echo CREATING ROLE BINDINGS
  oc create clusterrolebinding cluster-manager-admin-binding --clusterrole=open-cluster-management:cluster-manager-admin --user=admin
  oc create clusterrolebinding edit-binding --clusterrole=edit --user=user1
  oc create clusterrolebinding view-binding --clusterrole=view --user=user2
}

collect_users_oc_token() {
  echo COLLECTING USER OC TOKENS
  oc login -u admin -p admin
  ADMIN_TOKEN=$(oc whoami -t)
  oc login -u user1 -p user1
  USER1_TOKEN=$(oc whoami -t)
  oc login -u user2 -p user2
  USER2_TOKEN=$(oc whoami -t)
}

if ! which htpasswd &>/dev/null; then
  if which apt-get &>/dev/null; then
    sudo apt-get update
    sudo apt-get install -y apache2-utils
  else
    echo "Error: Package manager apt-get not found. Failed to find or install htpasswd."
    exit 1
  fi
fi

create_test_users
create_auth_provider
create_role_bindings
collect_users_oc_token
