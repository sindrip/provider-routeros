package v1alpha1

import (
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UnlistedPolicy controls device rows that are not represented by spec.rows.
// +kubebuilder:validation:Enum=Tolerate;Prune
type UnlistedPolicy string

const (
	UnlistedTolerate UnlistedPolicy = "Tolerate"
	UnlistedPrune    UnlistedPolicy = "Prune"
)

// DeletionPolicy controls device rows when the Kubernetes object is deleted.
// Orphan is deliberately the default: deleting a Kubernetes object must not
// implicitly wipe a router's firewall.
// +kubebuilder:validation:Enum=Orphan;Delete
type DeletionPolicy string

const (
	DeletionOrphan DeletionPolicy = "Orphan"
	DeletionDelete DeletionPolicy = "Delete"
)

// ProviderConfigReference names a cluster-scoped routeros.sindrip.io
// ProviderConfig.
type ProviderConfigReference struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// FirewallFilterMenuSpec declares the ordered contents of
// /ip/firewall/filter on one router.
// +kubebuilder:validation:XValidation:rule="!has(self.deletionPolicy) || self.deletionPolicy != 'Delete' || self.unlisted == 'Prune'",message="deletionPolicy Delete requires unlisted Prune because only a full-menu owner may empty the menu"
type FirewallFilterMenuSpec struct {
	ProviderConfigRef ProviderConfigReference `json:"providerConfigRef"`

	// Unlisted is required because both choices have material consequences:
	// Tolerate preserves hand-written rules; Prune makes this object own the
	// complete menu.
	Unlisted UnlistedPolicy `json:"unlisted"`

	// DeletionPolicy defaults to Orphan so removing this object leaves RouterOS
	// configuration intact. Delete is valid only with Unlisted=Prune, where the
	// object explicitly owns the complete menu.
	// +kubebuilder:default=Orphan
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

	// Rows are evaluated in list order by RouterOS. The list is atomic so
	// server-side apply cannot merge rules from multiple field managers into a
	// surprising first-match chain.
	// +listType=atomic
	Rows []FirewallFilterRule `json:"rows"`
}

// FirewallFilterRule is the writable RouterOS 7.23.2 surface discovered by
// the IR for /ip/firewall/filter. Pointer fields preserve the distinction
// between an omitted field and an explicitly empty/false value.
//
// RouterOS accepts several scalar mini-languages (ranges, comma-separated
// sets, negation, and time units), so non-boolean values remain strings at the
// API boundary instead of being narrowed into lossy Go numeric types.
type FirewallFilterRule struct {
	Action                  *string `json:"action,omitempty"`
	AddressList             *string `json:"addressList,omitempty"`
	AddressListTimeout      *string `json:"addressListTimeout,omitempty"`
	Chain                   *string `json:"chain,omitempty"`
	Comment                 *string `json:"comment,omitempty"`
	ConnectionBytes         *string `json:"connectionBytes,omitempty"`
	ConnectionLimit         *string `json:"connectionLimit,omitempty"`
	ConnectionMark          *string `json:"connectionMark,omitempty"`
	ConnectionNatState      *string `json:"connectionNatState,omitempty"`
	ConnectionRate          *string `json:"connectionRate,omitempty"`
	ConnectionState         *string `json:"connectionState,omitempty"`
	ConnectionType          *string `json:"connectionType,omitempty"`
	Content                 *string `json:"content,omitempty"`
	Disabled                *bool   `json:"disabled,omitempty"`
	Dscp                    *string `json:"dscp,omitempty"`
	DstAddress              *string `json:"dstAddress,omitempty"`
	DstAddressList          *string `json:"dstAddressList,omitempty"`
	DstAddressType          *string `json:"dstAddressType,omitempty"`
	DstLimit                *string `json:"dstLimit,omitempty"`
	DstPort                 *string `json:"dstPort,omitempty"`
	Fragment                *bool   `json:"fragment,omitempty"`
	Hotspot                 *string `json:"hotspot,omitempty"`
	IcmpOptions             *string `json:"icmpOptions,omitempty"`
	InBridgePort            *string `json:"inBridgePort,omitempty"`
	InBridgePortList        *string `json:"inBridgePortList,omitempty"`
	InInterface             *string `json:"inInterface,omitempty"`
	InInterfaceList         *string `json:"inInterfaceList,omitempty"`
	IngressPriority         *string `json:"ingressPriority,omitempty"`
	IpsecPolicy             *string `json:"ipsecPolicy,omitempty"`
	Ipv4Options             *string `json:"ipv4Options,omitempty"`
	JumpTarget              *string `json:"jumpTarget,omitempty"`
	Layer7Protocol          *string `json:"layer7Protocol,omitempty"`
	Limit                   *string `json:"limit,omitempty"`
	Log                     *bool   `json:"log,omitempty"`
	LogPrefix               *string `json:"logPrefix,omitempty"`
	Nth                     *string `json:"nth,omitempty"`
	OutBridgePort           *string `json:"outBridgePort,omitempty"`
	OutBridgePortList       *string `json:"outBridgePortList,omitempty"`
	OutInterface            *string `json:"outInterface,omitempty"`
	OutInterfaceList        *string `json:"outInterfaceList,omitempty"`
	P2P                     *string `json:"p2p,omitempty"`
	PacketMark              *string `json:"packetMark,omitempty"`
	PacketSize              *string `json:"packetSize,omitempty"`
	PerConnectionClassifier *string `json:"perConnectionClassifier,omitempty"`
	Port                    *string `json:"port,omitempty"`
	Priority                *string `json:"priority,omitempty"`
	Protocol                *string `json:"protocol,omitempty"`
	PSD                     *string `json:"psd,omitempty"`
	Random                  *string `json:"random,omitempty"`
	Realm                   *string `json:"realm,omitempty"`
	RejectWith              *string `json:"rejectWith,omitempty"`
	RoutingMark             *string `json:"routingMark,omitempty"`
	SrcAddress              *string `json:"srcAddress,omitempty"`
	SrcAddressList          *string `json:"srcAddressList,omitempty"`
	SrcAddressType          *string `json:"srcAddressType,omitempty"`
	SrcMACAddress           *string `json:"srcMacAddress,omitempty"`
	SrcPort                 *string `json:"srcPort,omitempty"`
	TCPFlags                *string `json:"tcpFlags,omitempty"`
	TCPMSS                  *string `json:"tcpMss,omitempty"`
	Time                    *string `json:"time,omitempty"`
	TLSHost                 *string `json:"tlsHost,omitempty"`
	TOS                     *string `json:"tos,omitempty"`
	TTL                     *string `json:"ttl,omitempty"`
}

// Fields returns exactly the values declared by the user, translated to
// RouterOS REST field names. In particular, explicit false is retained.
func (r FirewallFilterRule) Fields() map[string]string {
	out := map[string]string{}
	putString := func(name string, value *string) {
		if value != nil {
			out[name] = *value
		}
	}
	putBool := func(name string, value *bool) {
		if value != nil {
			out[name] = strconv.FormatBool(*value)
		}
	}

	putString("action", r.Action)
	putString("address-list", r.AddressList)
	putString("address-list-timeout", r.AddressListTimeout)
	putString("chain", r.Chain)
	putString("comment", r.Comment)
	putString("connection-bytes", r.ConnectionBytes)
	putString("connection-limit", r.ConnectionLimit)
	putString("connection-mark", r.ConnectionMark)
	putString("connection-nat-state", r.ConnectionNatState)
	putString("connection-rate", r.ConnectionRate)
	putString("connection-state", r.ConnectionState)
	putString("connection-type", r.ConnectionType)
	putString("content", r.Content)
	putBool("disabled", r.Disabled)
	putString("dscp", r.Dscp)
	putString("dst-address", r.DstAddress)
	putString("dst-address-list", r.DstAddressList)
	putString("dst-address-type", r.DstAddressType)
	putString("dst-limit", r.DstLimit)
	putString("dst-port", r.DstPort)
	putBool("fragment", r.Fragment)
	putString("hotspot", r.Hotspot)
	putString("icmp-options", r.IcmpOptions)
	putString("in-bridge-port", r.InBridgePort)
	putString("in-bridge-port-list", r.InBridgePortList)
	putString("in-interface", r.InInterface)
	putString("in-interface-list", r.InInterfaceList)
	putString("ingress-priority", r.IngressPriority)
	putString("ipsec-policy", r.IpsecPolicy)
	putString("ipv4-options", r.Ipv4Options)
	putString("jump-target", r.JumpTarget)
	putString("layer7-protocol", r.Layer7Protocol)
	putString("limit", r.Limit)
	putBool("log", r.Log)
	putString("log-prefix", r.LogPrefix)
	putString("nth", r.Nth)
	putString("out-bridge-port", r.OutBridgePort)
	putString("out-bridge-port-list", r.OutBridgePortList)
	putString("out-interface", r.OutInterface)
	putString("out-interface-list", r.OutInterfaceList)
	putString("p2p", r.P2P)
	putString("packet-mark", r.PacketMark)
	putString("packet-size", r.PacketSize)
	putString("per-connection-classifier", r.PerConnectionClassifier)
	putString("port", r.Port)
	putString("priority", r.Priority)
	putString("protocol", r.Protocol)
	putString("psd", r.PSD)
	putString("random", r.Random)
	putString("realm", r.Realm)
	putString("reject-with", r.RejectWith)
	putString("routing-mark", r.RoutingMark)
	putString("src-address", r.SrcAddress)
	putString("src-address-list", r.SrcAddressList)
	putString("src-address-type", r.SrcAddressType)
	putString("src-mac-address", r.SrcMACAddress)
	putString("src-port", r.SrcPort)
	putString("tcp-flags", r.TCPFlags)
	putString("tcp-mss", r.TCPMSS)
	putString("time", r.Time)
	putString("tls-host", r.TLSHost)
	putString("tos", r.TOS)
	putString("ttl", r.TTL)
	return out
}

// FirewallFilterRowStatus identifies the RouterOS row matched to one desired
// list position without mirroring the full spec into status.
type FirewallFilterRowStatus struct {
	Index int32  `json:"index"`
	ID    string `json:"id,omitempty"`
}

// FirewallFilterPlanStatus is a compact preview of a first destructive prune.
// ApprovalToken changes whenever either the connection or planned operations
// change, preventing approval from being reused for a different router state.
type FirewallFilterPlanStatus struct {
	ApprovalToken string `json:"approvalToken"`
	Creates       int32  `json:"creates,omitempty"`
	Updates       int32  `json:"updates,omitempty"`
	Deletes       int32  `json:"deletes,omitempty"`
	Moves         int32  `json:"moves,omitempty"`
	// DeleteRows previews up to twenty static rows that would be removed.
	// +listType=atomic
	DeleteRows []FirewallFilterDeletePreview `json:"deleteRows,omitempty"`
	// DeleteRowsTruncated is true when Deletes is larger than DeleteRows.
	DeleteRowsTruncated bool `json:"deleteRowsTruncated,omitempty"`
}

// FirewallFilterDeletePreview identifies a row in a destructive adoption plan
// without copying the complete firewall rule into Kubernetes status.
type FirewallFilterDeletePreview struct {
	ID      string `json:"id"`
	Chain   string `json:"chain,omitempty"`
	Action  string `json:"action,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// FirewallFilterMenuStatus is intentionally compact: the desired values
// already live in spec, so status reports resolution, health, and only a
// bounded summary when destructive adoption is pending.
type FirewallFilterMenuStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Adopted is true after the first prune for the current connection has
	// either been approved or was proven non-destructive.
	Adopted bool `json:"adopted,omitempty"`
	// AdoptedConnection is the non-secret fingerprint of the ProviderConfig and
	// Secret revision that was adopted.
	AdoptedConnection string `json:"adoptedConnection,omitempty"`
	// PendingPlan is present while the controller is waiting for approval of a
	// first prune that would delete existing static rows.
	PendingPlan *FirewallFilterPlanStatus `json:"pendingPlan,omitempty"`

	// +listType=atomic
	Rows []FirewallFilterRowStatus `json:"rows,omitempty"`

	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,provider,routeros}
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="POLICY",type=string,JSONPath=`.spec.unlisted`
// +kubebuilder:printcolumn:name="ADOPTED",type=boolean,JSONPath=`.status.adopted`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=`.metadata.creationTimestamp`

// FirewallFilterMenu owns the ordered /ip/firewall/filter menu on one RouterOS
// device.
type FirewallFilterMenu struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FirewallFilterMenuSpec   `json:"spec"`
	Status FirewallFilterMenuStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FirewallFilterMenuList contains FirewallFilterMenu objects.
type FirewallFilterMenuList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FirewallFilterMenu `json:"items"`
}
