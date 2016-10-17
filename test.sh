#!/bin/bash
docker build -t calico/node-libnetwork-go libnetwork-go
LOCAL_IP_ENV=`ip route get 8.8.8.8 | head -1 | cut -d' ' -f8`
docker run --detach \
--net=host \
--name calico-etcd quay.io/coreos/etcd:v2.0.11 \
--advertise-client-urls "http://$LOCAL_IP_ENV:2379,http://127.0.0.1:2379" \
--listen-client-urls "http://0.0.0.0:2379"

docker run -d --net=host --privileged \
-e CALICO_ETCD_AUTHORITY=$LOCAL_IP_ENV:2379 \
-v /run/docker/plugins:/run/docker/plugins \
-v /var/run/docker.sock:/var/run/docker.sock \
-v /lib/modules:/lib/modules \
-v $PWD/libnetwork-go:/go/src/github.com/libnetwork-plugin/libnetwork-go \
--name calico-node-libnetwork-go calico/node-libnetwork-go


docker run -d --restart=always --net=host --privileged --name=calico-node \
-e HOSTNAME=9411ddfe1fc9 \
-e IP=$LOCAL_IP_ENV \
-e IP6= \
-e CALICO_NETWORKING_BACKEND=bird \
-e AS= \
-e NO_DEFAULT_POOLS= \
-e CALICO_LIBNETWORK_ENABLED=false \
-e ETCD_AUTHORITY=$LOCAL_IP_ENV:2379 \
-e ETCD_SCHEME=http \
-v /var/log/calico:/var/log/calico \
-v /lib/modules:/lib/modules \
-v /var/run/calico:/var/run/calico \
calico/node:latest

docker network create -d calico-net  --subnet 192.168.0.0/16 testnet