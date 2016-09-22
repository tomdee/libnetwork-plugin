package responses

var (
	// Plugin activation, we activate both libnetwork and ipam plugins.
	PluginActivate = map[string][]string{
		"Implements": []string{"NetworkDriver", "IpamDriver"},
	}

	// Return fixed local and global address spaces.  The Calico IPAM module
    	// does not use the address space when assigning IP addresses.  Instead
    	// we assign from the pre-defined Calico IP pools.
	GetDefaultAddressSpaces = map[string]string{
		"LocalDefaultAddressSpace": "CalicoLocalAddressSpace",
        	"GlobalDefaultAddressSpace": "CalicoGlobalAddressSpace",
	}
)

type (
	IPAMDriverRequestPoolResponse struct {
		PoolID, Pool string
		Data map[string]string
	}
)
