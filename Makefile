SRC_FILES=$(shell find . -type f -name '*.go')

# These variables can be overridden by setting an environment variable.
LOCAL_IP_ENV?=$(shell ip route get 8.8.8.8 | head -1 | cut -d' ' -f8)
ST_TO_RUN?=tests/st
# Can exclude the slower tests with "-a '!slow'"
ST_OPTIONS?=
HOST_CHECKOUT_DIR?=$(shell pwd)

default: all
all: test
test: st

calico/node-libnetwork: caliconode.created

vendor:
	glide install -strip-vendor

install:
	CGO_ENABLED=0 go install github.com/projectcalico/libnetwork-plugin

caliconode.created: $(SRC_FILES) Dockerfile
	CGO_ENABLED=0 go build
	docker build -t calico/node-libnetwork .
	touch caliconode.created

dist/calicoctl:
	mkdir dist
	curl -L http://www.projectcalico.org/builds/calicoctl -o dist/calicoctl
	chmod +x dist/calicoctl

busybox.tar:
	docker pull busybox:latest
	docker save busybox:latest -o busybox.tar

calico-node.tar:
	docker pull calico/node:latest
	docker save calico/node:latest -o calico-node.tar

calico-node-libnetwork.tar: caliconode.created
	docker save calico/node-libnetwork:latest -o calico-node-libnetwork.tar

st:  dist/calicoctl busybox.tar calico-node.tar calico-node-libnetwork.tar run-etcd
	# Use the host, PID and network namespaces from the host.
	# Privileged is needed since 'calico node' write to /proc (to enable ip_forwarding)
	# Map the docker socket in so docker can be used from inside the container
	# HOST_CHECKOUT_DIR is used for volume mounts on containers started by this one.
	# All of code under test is mounted into the container.
	#   - This also provides access to calicoctl and the docker client
	docker run --uts=host \
	           --pid=host \
	           --net=host \
	           --privileged \
	           -e HOST_CHECKOUT_DIR=$(HOST_CHECKOUT_DIR) \
	           -e DEBUG_FAILURES=$(DEBUG_FAILURES) \
	           --rm -ti \
	           -v /var/run/docker.sock:/var/run/docker.sock \
	           -v `pwd`:/code \
	           calico/test \
	           sh -c 'cp -ra tests/st/libnetwork/ /tests/st && cd / && nosetests $(ST_TO_RUN) -sv --nologcapture --with-timer $(ST_OPTIONS)'

run-plugin: caliconode.created
	docker run --rm --net=host --privileged -e CALICO_ETCD_AUTHORITY=$(LOCAL_IP_ENV):2379 -v /run/docker/plugins:/run/docker/plugins -v /var/run/docker.sock:/var/run/docker.sock -v /lib/modules:/lib/modules --name calico-node-libnetwork calico/node-libnetwork /calico


run-etcd:
	@-docker rm -f calico-etcd calico-etcd-ssl
	docker run --detach \
	--net=host \
	--name calico-etcd quay.io/coreos/etcd:v2.0.11 \
	--advertise-client-urls "http://$(LOCAL_IP_ENV):2379,http://127.0.0.1:2379" \
	--listen-client-urls "http://0.0.0.0:2379"

semaphore:
	docker version

	# Ensure Semaphore has loaded the required modules
	modprobe -a ip6_tables xt_set

	# Run the STs
	make st

	# Run subset of STs with secure etcd (only a few total, so just run all of them)
	# Temporarily disable the secure STs
	make st-ssl

clean:
	-rm -f *.created
	-rm -rf dist
	-rm -rf certs
	-rm -f *.tar
	-rm -rf vendor
	-docker run -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/docker:/var/lib/docker --rm martin/docker-cleanup-volumes

## All SSL stuff is below - it may be removed in future.

## Run the STs in a container using etcd with SSL certificate/key/CA verification.
st-ssl: dist/calicoctl busybox.tar calico-node.tar calico-node-libnetwork.tar run-etcd-ssl
	# Use the host, PID and network namespaces from the host.
        # Privileged is needed since 'calico node' write to /proc (to enable ip_forwarding)
        # Map the docker socket in so docker can be used from inside the container
        # HOST_CHECKOUT_DIR is used for volume mounts on containers started by this one.
        # All of code under test is mounted into the container.
        #   - This also provides access to calicoctl and the docker client
        # Mount the full path to the etcd certs directory.
        #   - docker copies this directory directly from the host, but the
        #     calicoctl node command reads the files from the test container
	docker run --uts=host \
	           --pid=host \
	           --net=host \
	           --privileged \
	           -e HOST_CHECKOUT_DIR=$(HOST_CHECKOUT_DIR) \
	           -e ETCD_SCHEME=https \
	           -e ETCD_CA_CERT_FILE=`pwd`/certs/ca.pem \
	           -e ETCD_CERT_FILE=`pwd`/certs/client.pem \
	           -e ETCD_KEY_FILE=`pwd`/certs/client-key.pem \
	           -e DEBUG_FAILURES=$(DEBUG_FAILURES) \
	           --rm -ti \
	           -v /var/run/docker.sock:/var/run/docker.sock \
	           -v `pwd`:/code \
	           -v `pwd`/certs:`pwd`/certs \
	           calico/test \
	           sh -c 'cp -ra tests/st/* /tests/st && cd / && nosetests $(ST_TO_RUN) -sv --nologcapture --with-timer $(ST_OPTIONS)'

add-ssl-hostname:
	# Set "LOCAL_IP etcd-authority-ssl" in /etc/hosts to use as a hostname for etcd with ssl
	if ! grep -q "etcd-authority-ssl" /etc/hosts; then \
	  echo "\n# Host used by Calico's ETCD with SSL\n$(LOCAL_IP_ENV) etcd-authority-ssl" >> /etc/hosts; \
	fi

## Run etcd in a container with SSL verification. Used primarily by STs.
run-etcd-ssl: certs/.certificates.created add-ssl-hostname
	@-docker rm -f calico-etcd calico-etcd-ssl
	docker run --detach \
	--net=host \
	-v `pwd`/certs:/etc/calico/certs \
	--name calico-etcd-ssl quay.io/coreos/etcd:v2.0.11 \
	--cert-file "/etc/calico/certs/server.pem" \
	--key-file "/etc/calico/certs/server-key.pem" \
	--ca-file "/etc/calico/certs/ca.pem" \
	--advertise-client-urls "https://etcd-authority-ssl:2379,https://localhost:2379" \
	--listen-client-urls "https://0.0.0.0:2379"

## Generate the keys and certificates for running etcd with SSL.
certs/.certificates.created:
	mkdir -p certs
	curl -L "https://github.com/projectcalico/cfssl/releases/download/1.2.1/cfssl" -o certs/cfssl
	curl -L "https://github.com/projectcalico/cfssl/releases/download/1.2.1/cfssljson" -o certs/cfssljson
	chmod a+x certs/cfssl
	chmod a+x certs/cfssljson

	certs/cfssl gencert -initca tests/st/ssl-config/ca-csr.json | certs/cfssljson -bare certs/ca
	certs/cfssl gencert \
	  -ca certs/ca.pem \
	  -ca-key certs/ca-key.pem \
	  -config tests/st/ssl-config/ca-config.json \
	  tests/st/ssl-config/req-csr.json | certs/cfssljson -bare certs/client
	certs/cfssl gencert \
	  -ca certs/ca.pem \
	  -ca-key certs/ca-key.pem \
	  -config tests/st/ssl-config/ca-config.json \
	  tests/st/ssl-config/req-csr.json | certs/cfssljson -bare certs/server

	touch certs/.certificates.created

ssl-certs: certs/.certificates.created ## Generate self-signed SSL certificates
