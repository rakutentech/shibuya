#!/bin/bash
kubeconfig="apiVersion: v1
kind: Config
users:
- name: shibuya-user
  user:
    token: TOKEN_HERE
clusters:
- cluster:
    certificate-authority-data: CA_HERE
    server: SERVER_HERE
  name: shibuya-cluster
contexts:
- context:
    cluster: shibuya-cluster
    user: shibuya-user
  name: shibuya-context
current-context: shibuya-context"

# check for namespace
if [ "$1" == "" ]; then
    echo "Provide namespace where shibuya service account exists"
    exit 1
fi

if [ "$2" == "" ]; then
    echo "Provide the service account secret name"
    exit 1
fi


# get token from secret
TOKEN=$(kubectl -n $1 get secrets $2 -o=custom-columns=:.data.token)
TOKEN=$(echo $TOKEN | base64 -d)
kubeconfig=$(echo "$kubeconfig" | sed 's,TOKEN_HERE,'"$TOKEN"',g')

# get ca.crt from secret
CAcrt=$(kubectl -n $1 get secrets $2 -o=custom-columns=:.data."ca\.crt" | tr -d '\n')
kubeconfig=$(echo "$kubeconfig" | sed 's,CA_HERE,'"$CAcrt"',g')

# get API server master url
SERVER=$(TERM=dumb kubectl cluster-info | grep "Kubernetes control" | awk '{print $NF}')
kubeconfig=$(echo "$kubeconfig" | sed 's,SERVER_HERE,'"$SERVER"',g')

# export kubeconfig to shibuya/config/kube_configs using context name
FILEPATH="shibuya/config/kube_configs/$(kubectl config current-context)"
echo "$kubeconfig" > "$FILEPATH"
