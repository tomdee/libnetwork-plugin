// Copyright (c) 2016 Tigera, Inc. All rights reserved.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"github.com/tigera/libcalico-go/lib/api/unversioned"
	"github.com/tigera/libcalico-go/lib/net"
	"github.com/tigera/libcalico-go/lib/scope"
)

// BGPPeer contains information about a BGP peer resource that is a peer of a Calico
// compute node.
type BGPPeer struct {
	unversioned.TypeMetadata

	// Metadata for a BGPPeer.
	Metadata BGPPeerMetadata `json:"metadata,omitempty"`

	// Specification for a BGPPeer.
	Spec BGPPeerSpec `json:"spec,omitempty"`
}

// BGPPeerMetadata contains the metadata for a BGPPeer resource.
type BGPPeerMetadata struct {
	unversioned.ObjectMetadata

	// The scope of the peer.  This may be global or node.  A global peer is a
	// BGP device that peers with all Calico nodes.  A node peer is a BGP device that
	// peers with the specified Calico node (specified by the node hostname).
	Scope scope.Scope `json:"scope" validate:"omitempty,scopeglobalornode"`

	// The hostname of the compute server that is peering with this peer.  When modifying a
	// BGP peer, the hostname must be specified when the scope is `node`, and must
	// be omitted when the scope is `global`.
	Hostname string `json:"hostname,omitempty" validate:"omitempty,name"`

	// The IP address of the peer.
	PeerIP net.IP `json:"peerIP" validate:"omitempty,ip"`
}

// BGPPeerSpec contains the specification for a BGPPeer resource.
type BGPPeerSpec struct {
	// The AS Number of the peer.
	ASNumber int `json:"asNumber" validate:"required,asn"`
}

// NewBGPPeer creates a new (zeroed) BGPPeer struct with the TypeMetadata initialised to the current
// version.
func NewBGPPeer() *BGPPeer {
	return &BGPPeer{
		TypeMetadata: unversioned.TypeMetadata{
			Kind: "bgpPeer",
			APIVersion: unversioned.VersionCurrent,
		},
	}
}

// BGPPeerList contains a list of BGP Peer resources.  List types are returned from List()
// enumerations in the client interface.
type BGPPeerList struct {
	unversioned.TypeMetadata
	Metadata unversioned.ListMetadata `json:"metadata,omitempty"`
	Items    []BGPPeer    `json:"items" validate:"dive"`
}

// NewBGPPeerList creates a new (zeroed) BGPPeerList struct with the TypeMetadata initialised to the current
// version.
func NewBGPPeerList() *BGPPeerList {
	return &BGPPeerList{
		TypeMetadata: unversioned.TypeMetadata{
			Kind: "bgpPeerList",
			APIVersion: unversioned.VersionCurrent,
		},
	}
}
