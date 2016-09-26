FROM golang:1.7.1

MAINTAINER Dmitry Yakutkin <dmitry.yakutkin@gmail.com>

ENV PLUGIN_SERVER_PORT 9000
ADD . /go/src/github.com/libnetwork-plugin/libnetwork-go
WORKDIR /go/src/github.com/libnetwork-plugin/libnetwork-go
CMD /bin/bash -c "go run main.go"
