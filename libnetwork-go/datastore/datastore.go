package datastore

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/pkg/transport"
	"github.com/docker/go-plugins-helpers/network"
	"github.com/projectcalico/libcalico-go/lib/api"
	calicoEtcd "github.com/projectcalico/libcalico-go/lib/backend/etcd"
	"golang.org/x/net/context"
)

const (
	clientTimeout = 30 * time.Second
	etcdPrefix    = "/v2/keys/calico/libnetwork/v1/"
)

type Datastore interface {
	GetNetwork(networkID string) (*Network, error)
	WriteNetwork(networkID string, data Network) error
	RemoveNetwork(networkID string) error
}

type Network struct {
	NetworkID string
	Options   map[string]interface{}
	IPv4Data  []*network.IPAMData
	IPv6Data  []*network.IPAMData
}

type CalicoDatastore struct {
	etcd client.KeysAPI
}

// New is the only way CalicoDatastore instances should be created.
// It basically recreates etcd client which will be used in libcalico-go's client
// based on given config, but makes it available to direct use.
// Code is borrowed from calico api client constructor.
func New(config api.ClientConfig) (Datastore, error) {
	etcdConfig, ok := config.BackendConfig.(*calicoEtcd.EtcdConfig)

	if !ok {
		return nil, errors.New("Invalid config format")
	}

	// Determine the location from the authority or the endpoints. The endpoints
	// takes precedence if both are specified.
	etcdLocation := []string{}
	if etcdConfig.EtcdAuthority != "" {
		etcdLocation = []string{etcdConfig.EtcdScheme + "://" + etcdConfig.EtcdAuthority}
	}
	if etcdConfig.EtcdEndpoints != "" {
		etcdLocation = strings.Split(etcdConfig.EtcdEndpoints, ",")
	}

	if len(etcdLocation) == 0 {
		return nil, errors.New("no etcd authority or endpoints specified")
	}

	// Create the etcd client
	tls := transport.TLSInfo{
		CAFile:   etcdConfig.EtcdCACertFile,
		CertFile: etcdConfig.EtcdCertFile,
		KeyFile:  etcdConfig.EtcdKeyFile,
	}
	transport, err := transport.NewTransport(tls, clientTimeout)
	if err != nil {
		return nil, err
	}

	cfg := client.Config{
		Endpoints:               etcdLocation,
		Transport:               transport,
		HeaderTimeoutPerRequest: clientTimeout,
	}

	if etcdConfig.EtcdUsername != "" && etcdConfig.EtcdPassword != "" {
		cfg.Username = etcdConfig.EtcdUsername
		cfg.Password = etcdConfig.EtcdPassword
	}

	etcdClient, err := client.New(cfg)
	if err != nil {
		return nil, err
	}
	keys := client.NewKeysAPIWithPrefix(etcdClient, etcdPrefix)

	return CalicoDatastore{keys}, nil
}

func (d CalicoDatastore) GetNetwork(networkID string) (*Network, error) {
	resp, err := d.etcd.Get(context.Background(), networkID, nil)
	if err != nil {
		return nil, err
	}
	var result *Network
	if err = json.Unmarshal([]byte(resp.Node.Value), result); err != nil {
		return nil, err
	}
	return result, nil
}

func (d CalicoDatastore) WriteNetwork(networkID string, data Network) error {
	marshaledData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = d.etcd.Set(context.Background(), networkID, string(marshaledData), nil)
	return err
}

func (d CalicoDatastore) RemoveNetwork(networkID string) error {
	_, err := d.etcd.Delete(context.Background(), networkID, nil)
	return err
}
