package responses

var (
	PluginActivate = map[string][]string{
		"Implements": []string{"NetworkDriver", "IpamDriver"},
	}
	GetDefaultAddressSpaces = map[string]string{
		"LocalDefaultAddressSpace": "CalicoLocalAddressSpace",
        	"GlobalDefaultAddressSpace": "CalicoGlobalAddressSpace",
	}
)
