// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/upjet/v2/pkg/controller"

	mlag "github.com/sindrip/provider-routeros/internal/controller/namespaced/bridge/mlag"
	port "github.com/sindrip/provider-routeros/internal/controller/namespaced/bridge/port"
	vlan "github.com/sindrip/provider-routeros/internal/controller/namespaced/bridge/vlan"
	aaa "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/aaa"
	accesslist "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/accesslist"
	capsmaninterface "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/capsmaninterface"
	channel "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/channel"
	configuration "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/configuration"
	datapath "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/datapath"
	manager "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/manager"
	managerinterface "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/managerinterface"
	provisioning "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/provisioning"
	rates "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/rates"
	security "github.com/sindrip/provider-routeros/internal/controller/namespaced/capsman/security"
	scepserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/certificate/scepserver"
	config "github.com/sindrip/provider-routeros/internal/controller/namespaced/container/config"
	envs "github.com/sindrip/provider-routeros/internal/controller/namespaced/container/envs"
	mounts "github.com/sindrip/provider-routeros/internal/controller/namespaced/container/mounts"
	client "github.com/sindrip/provider-routeros/internal/controller/namespaced/dhcp/client"
	clientoption "github.com/sindrip/provider-routeros/internal/controller/namespaced/dhcp/clientoption"
	server "github.com/sindrip/provider-routeros/internal/controller/namespaced/dhcp/server"
	serverlease "github.com/sindrip/provider-routeros/internal/controller/namespaced/dhcp/serverlease"
	servernetwork "github.com/sindrip/provider-routeros/internal/controller/namespaced/dhcp/servernetwork"
	settings "github.com/sindrip/provider-routeros/internal/controller/namespaced/disk/settings"
	record "github.com/sindrip/provider-routeros/internal/controller/namespaced/dns/record"
	addrlist "github.com/sindrip/provider-routeros/internal/controller/namespaced/firewall/addrlist"
	filter "github.com/sindrip/provider-routeros/internal/controller/namespaced/firewall/filter"
	mangle "github.com/sindrip/provider-routeros/internal/controller/namespaced/firewall/mangle"
	nat "github.com/sindrip/provider-routeros/internal/controller/namespaced/firewall/nat"
	bonding "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/bonding"
	bridge "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/bridge"
	bridgefilter "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/bridgefilter"
	bridgeport "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/bridgeport"
	bridgesettings "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/bridgesettings"
	bridgevlan "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/bridgevlan"
	detectinternet "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/detectinternet"
	dot1xclient "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/dot1xclient"
	dot1xserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/dot1xserver"
	eoip "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/eoip"
	ethernet "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernet"
	ethernetswitch "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitch"
	ethernetswitchcrs "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchcrs"
	ethernetswitchcrsegressvlantag "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchcrsegressvlantag"
	ethernetswitchcrsegressvlantranslation "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchcrsegressvlantranslation"
	ethernetswitchcrsingressvlantranslation "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchcrsingressvlantranslation"
	ethernetswitchcrsvlan "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchcrsvlan"
	ethernetswitchhost "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchhost"
	ethernetswitchport "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchport"
	ethernetswitchportisolation "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchportisolation"
	ethernetswitchrule "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchrule"
	ethernetswitchvlan "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ethernetswitchvlan"
	gre "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/gre"
	gre6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/gre6"
	ipip "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ipip"
	l2tpclient "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/l2tpclient"
	list "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/list"
	listmember "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/listmember"
	lte "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/lte"
	lteapn "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/lteapn"
	macvlan "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/macvlan"
	ovpnclient "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ovpnclient"
	ovpnserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/ovpnserver"
	pppoeclient "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/pppoeclient"
	pppoeserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/pppoeserver"
	sixtofour "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/sixtofour"
	sstpclient "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/sstpclient"
	sstpserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/sstpserver"
	veth "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/veth"
	vlaninterface "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/vlan"
	vrrp "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/vrrp"
	vxlan "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/vxlan"
	vxlanvteps "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/vxlanvteps"
	w60g "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/w60g"
	w60gstation "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/w60gstation"
	wireguard "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/wireguard"
	wireguardpeer "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/wireguardpeer"
	wireless "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/wireless"
	wirelessaccesslist "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/wirelessaccesslist"
	wirelesscap "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/wirelesscap"
	wirelessconnectlist "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/wirelessconnectlist"
	wirelesssecurityprofiles "github.com/sindrip/provider-routeros/internal/controller/namespaced/interface/wirelesssecurityprofiles"
	address "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/address"
	cloud "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/cloud"
	cloudadvanced "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/cloudadvanced"
	dhcpclient "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpclient"
	dhcpclientoption "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpclientoption"
	dhcprelay "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcprelay"
	dhcpserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpserver"
	dhcpserverconfig "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpserverconfig"
	dhcpserverlease "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpserverlease"
	dhcpservernetwork "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpservernetwork"
	dhcpserveroption "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpserveroption"
	dhcpserveroptionmatcher "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpserveroptionmatcher"
	dhcpserveroptionset "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpserveroptionset"
	dhcpserveroptionsets "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dhcpserveroptionsets"
	dns "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dns"
	dnsadlist "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dnsadlist"
	dnsforwarders "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dnsforwarders"
	dnsrecord "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/dnsrecord"
	firewalladdrlist "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/firewalladdrlist"
	firewallconnectiontracking "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/firewallconnectiontracking"
	firewallfilter "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/firewallfilter"
	firewalllayer7protocol "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/firewalllayer7protocol"
	firewallmangle "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/firewallmangle"
	firewallnat "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/firewallnat"
	firewallraw "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/firewallraw"
	hotspot "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/hotspot"
	hotspotipbinding "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/hotspotipbinding"
	hotspotprofile "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/hotspotprofile"
	hotspotserviceport "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/hotspotserviceport"
	hotspotuser "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/hotspotuser"
	hotspotuserprofile "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/hotspotuserprofile"
	hotspotwalledgarden "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/hotspotwalledgarden"
	hotspotwalledgardenip "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/hotspotwalledgardenip"
	ipsecidentity "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipsecidentity"
	ipseckey "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipseckey"
	ipsecmodeconfig "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipsecmodeconfig"
	ipsecpeer "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipsecpeer"
	ipsecpolicy "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipsecpolicy"
	ipsecpolicygroup "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipsecpolicygroup"
	ipsecprofile "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipsecprofile"
	ipsecproposal "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipsecproposal"
	ipsecsettings "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/ipsecsettings"
	natpmp "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/natpmp"
	natpmpinterfaces "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/natpmpinterfaces"
	neighbordiscoverysettings "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/neighbordiscoverysettings"
	pool "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/pool"
	route "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/route"
	service "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/service"
	settingsip "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/settings"
	smb "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/smb"
	sshserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/sshserver"
	tftp "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/tftp"
	tftpsettings "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/tftpsettings"
	trafficflow "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/trafficflow"
	trafficflowipfix "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/trafficflowipfix"
	trafficflowtarget "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/trafficflowtarget"
	upnp "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/upnp"
	upnpinterfaces "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/upnpinterfaces"
	vrf "github.com/sindrip/provider-routeros/internal/controller/namespaced/ip/vrf"
	addressipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/address"
	dhcpclientipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/dhcpclient"
	dhcpclientoptionipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/dhcpclientoption"
	dhcpserveripv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/dhcpserver"
	dhcpserveroptionipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/dhcpserveroption"
	dhcpserveroptionsetsipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/dhcpserveroptionsets"
	firewalladdrlistipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/firewalladdrlist"
	firewallfilteripv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/firewallfilter"
	firewallmangleipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/firewallmangle"
	firewallnatipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/firewallnat"
	ndprefix "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/ndprefix"
	neighbordiscovery "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/neighbordiscovery"
	poolipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/pool"
	routeipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/route"
	settingsipv6 "github.com/sindrip/provider-routeros/internal/controller/namespaced/ipv6/settings"
	items "github.com/sindrip/provider-routeros/internal/controller/namespaced/move/items"
	serverovpn "github.com/sindrip/provider-routeros/internal/controller/namespaced/ovpn/server"
	aaappp "github.com/sindrip/provider-routeros/internal/controller/namespaced/ppp/aaa"
	profile "github.com/sindrip/provider-routeros/internal/controller/namespaced/ppp/profile"
	secret "github.com/sindrip/provider-routeros/internal/controller/namespaced/ppp/secret"
	providerconfig "github.com/sindrip/provider-routeros/internal/controller/namespaced/providerconfig"
	queuetype "github.com/sindrip/provider-routeros/internal/controller/namespaced/queue/queuetype"
	simple "github.com/sindrip/provider-routeros/internal/controller/namespaced/queue/simple"
	tree "github.com/sindrip/provider-routeros/internal/controller/namespaced/queue/tree"
	incoming "github.com/sindrip/provider-routeros/internal/controller/namespaced/radius/incoming"
	bridgerouteros "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/bridge"
	container "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/container"
	dnsrouteros "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/dns"
	file "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/file"
	grerouteros "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/gre"
	identity "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/identity"
	ipiprouteros "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/ipip"
	radius "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/radius"
	scheduler "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/scheduler"
	snmp "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/snmp"
	vlanrouteros "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/vlan"
	vrrprouteros "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/vrrp"
	wifi "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/wifi"
	wireguardrouteros "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/wireguard"
	zerotier "github.com/sindrip/provider-routeros/internal/controller/namespaced/routeros/zerotier"
	bfdconfiguration "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/bfdconfiguration"
	bgpconnection "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/bgpconnection"
	bgpevpn "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/bgpevpn"
	bgpinstance "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/bgpinstance"
	bgptemplate "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/bgptemplate"
	bgpvpn "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/bgpvpn"
	filterrule "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/filterrule"
	igmpproxyinterface "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/igmpproxyinterface"
	ospfarea "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/ospfarea"
	ospfarearange "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/ospfarearange"
	ospfinstance "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/ospfinstance"
	ospfinterfacetemplate "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/ospfinterfacetemplate"
	rule "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/rule"
	table "github.com/sindrip/provider-routeros/internal/controller/namespaced/routing/table"
	community "github.com/sindrip/provider-routeros/internal/controller/namespaced/snmp/community"
	certificate "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/certificate"
	certificatescepserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/certificatescepserver"
	clock "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/clock"
	identitysystem "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/identity"
	led "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/led"
	ledsettings "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/ledsettings"
	logging "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/logging"
	loggingaction "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/loggingaction"
	note "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/note"
	ntpclient "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/ntpclient"
	ntpserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/ntpserver"
	routerboardbuttonmode "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/routerboardbuttonmode"
	routerboardbuttonreset "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/routerboardbuttonreset"
	routerboardbuttonwps "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/routerboardbuttonwps"
	routerboardsettings "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/routerboardsettings"
	routerboardusb "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/routerboardusb"
	schedulersystem "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/scheduler"
	script "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/script"
	user "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/user"
	useraaa "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/useraaa"
	usergroup "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/usergroup"
	usersettings "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/usersettings"
	usersshkeys "github.com/sindrip/provider-routeros/internal/controller/namespaced/system/usersshkeys"
	bandwidthserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/bandwidthserver"
	email "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/email"
	graphinginterface "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/graphinginterface"
	graphingqueue "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/graphingqueue"
	graphingresource "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/graphingresource"
	macserver "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/macserver"
	macserverping "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/macserverping"
	macserverwinbox "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/macserverwinbox"
	netwatch "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/netwatch"
	sniffer "github.com/sindrip/provider-routeros/internal/controller/namespaced/tool/sniffer"
	manageradvanced "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/manageradvanced"
	managerattribute "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/managerattribute"
	managerdatabase "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/managerdatabase"
	managerlimitation "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/managerlimitation"
	managerprofile "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/managerprofile"
	managerprofilelimitation "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/managerprofilelimitation"
	managerrouter "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/managerrouter"
	managersettings "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/managersettings"
	manageruser "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/manageruser"
	managerusergroup "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/managerusergroup"
	manageruserprofile "github.com/sindrip/provider-routeros/internal/controller/namespaced/user/manageruserprofile"
	aaawifi "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/aaa"
	accesslistwifi "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/accesslist"
	cap "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/cap"
	capsman "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/capsman"
	channelwifi "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/channel"
	configurationwifi "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/configuration"
	datapathwifi "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/datapath"
	interworking "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/interworking"
	provisioningwifi "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/provisioning"
	securitywifi "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/security"
	securitymultipassphrase "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/securitymultipassphrase"
	steering "github.com/sindrip/provider-routeros/internal/controller/namespaced/wifi/steering"
	keys "github.com/sindrip/provider-routeros/internal/controller/namespaced/wireguard/keys"
	peer "github.com/sindrip/provider-routeros/internal/controller/namespaced/wireguard/peer"
	zerotiercontroller "github.com/sindrip/provider-routeros/internal/controller/namespaced/zerotier/zerotiercontroller"
	zerotierinterface "github.com/sindrip/provider-routeros/internal/controller/namespaced/zerotier/zerotierinterface"
)

// Setup creates all controllers with the supplied logger and adds them to
// the supplied manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		mlag.Setup,
		port.Setup,
		vlan.Setup,
		aaa.Setup,
		accesslist.Setup,
		capsmaninterface.Setup,
		channel.Setup,
		configuration.Setup,
		datapath.Setup,
		manager.Setup,
		managerinterface.Setup,
		provisioning.Setup,
		rates.Setup,
		security.Setup,
		scepserver.Setup,
		config.Setup,
		envs.Setup,
		mounts.Setup,
		client.Setup,
		clientoption.Setup,
		server.Setup,
		serverlease.Setup,
		servernetwork.Setup,
		settings.Setup,
		record.Setup,
		addrlist.Setup,
		filter.Setup,
		mangle.Setup,
		nat.Setup,
		bonding.Setup,
		bridge.Setup,
		bridgefilter.Setup,
		bridgeport.Setup,
		bridgesettings.Setup,
		bridgevlan.Setup,
		detectinternet.Setup,
		dot1xclient.Setup,
		dot1xserver.Setup,
		eoip.Setup,
		ethernet.Setup,
		ethernetswitch.Setup,
		ethernetswitchcrs.Setup,
		ethernetswitchcrsegressvlantag.Setup,
		ethernetswitchcrsegressvlantranslation.Setup,
		ethernetswitchcrsingressvlantranslation.Setup,
		ethernetswitchcrsvlan.Setup,
		ethernetswitchhost.Setup,
		ethernetswitchport.Setup,
		ethernetswitchportisolation.Setup,
		ethernetswitchrule.Setup,
		ethernetswitchvlan.Setup,
		gre.Setup,
		gre6.Setup,
		ipip.Setup,
		l2tpclient.Setup,
		list.Setup,
		listmember.Setup,
		lte.Setup,
		lteapn.Setup,
		macvlan.Setup,
		ovpnclient.Setup,
		ovpnserver.Setup,
		pppoeclient.Setup,
		pppoeserver.Setup,
		sixtofour.Setup,
		sstpclient.Setup,
		sstpserver.Setup,
		veth.Setup,
		vlaninterface.Setup,
		vrrp.Setup,
		vxlan.Setup,
		vxlanvteps.Setup,
		w60g.Setup,
		w60gstation.Setup,
		wireguard.Setup,
		wireguardpeer.Setup,
		wireless.Setup,
		wirelessaccesslist.Setup,
		wirelesscap.Setup,
		wirelessconnectlist.Setup,
		wirelesssecurityprofiles.Setup,
		address.Setup,
		cloud.Setup,
		cloudadvanced.Setup,
		dhcpclient.Setup,
		dhcpclientoption.Setup,
		dhcprelay.Setup,
		dhcpserver.Setup,
		dhcpserverconfig.Setup,
		dhcpserverlease.Setup,
		dhcpservernetwork.Setup,
		dhcpserveroption.Setup,
		dhcpserveroptionmatcher.Setup,
		dhcpserveroptionset.Setup,
		dhcpserveroptionsets.Setup,
		dns.Setup,
		dnsadlist.Setup,
		dnsforwarders.Setup,
		dnsrecord.Setup,
		firewalladdrlist.Setup,
		firewallconnectiontracking.Setup,
		firewallfilter.Setup,
		firewalllayer7protocol.Setup,
		firewallmangle.Setup,
		firewallnat.Setup,
		firewallraw.Setup,
		hotspot.Setup,
		hotspotipbinding.Setup,
		hotspotprofile.Setup,
		hotspotserviceport.Setup,
		hotspotuser.Setup,
		hotspotuserprofile.Setup,
		hotspotwalledgarden.Setup,
		hotspotwalledgardenip.Setup,
		ipsecidentity.Setup,
		ipseckey.Setup,
		ipsecmodeconfig.Setup,
		ipsecpeer.Setup,
		ipsecpolicy.Setup,
		ipsecpolicygroup.Setup,
		ipsecprofile.Setup,
		ipsecproposal.Setup,
		ipsecsettings.Setup,
		natpmp.Setup,
		natpmpinterfaces.Setup,
		neighbordiscoverysettings.Setup,
		pool.Setup,
		route.Setup,
		service.Setup,
		settingsip.Setup,
		smb.Setup,
		sshserver.Setup,
		tftp.Setup,
		tftpsettings.Setup,
		trafficflow.Setup,
		trafficflowipfix.Setup,
		trafficflowtarget.Setup,
		upnp.Setup,
		upnpinterfaces.Setup,
		vrf.Setup,
		addressipv6.Setup,
		dhcpclientipv6.Setup,
		dhcpclientoptionipv6.Setup,
		dhcpserveripv6.Setup,
		dhcpserveroptionipv6.Setup,
		dhcpserveroptionsetsipv6.Setup,
		firewalladdrlistipv6.Setup,
		firewallfilteripv6.Setup,
		firewallmangleipv6.Setup,
		firewallnatipv6.Setup,
		ndprefix.Setup,
		neighbordiscovery.Setup,
		poolipv6.Setup,
		routeipv6.Setup,
		settingsipv6.Setup,
		items.Setup,
		serverovpn.Setup,
		aaappp.Setup,
		profile.Setup,
		secret.Setup,
		providerconfig.Setup,
		queuetype.Setup,
		simple.Setup,
		tree.Setup,
		incoming.Setup,
		bridgerouteros.Setup,
		container.Setup,
		dnsrouteros.Setup,
		file.Setup,
		grerouteros.Setup,
		identity.Setup,
		ipiprouteros.Setup,
		radius.Setup,
		scheduler.Setup,
		snmp.Setup,
		vlanrouteros.Setup,
		vrrprouteros.Setup,
		wifi.Setup,
		wireguardrouteros.Setup,
		zerotier.Setup,
		bfdconfiguration.Setup,
		bgpconnection.Setup,
		bgpevpn.Setup,
		bgpinstance.Setup,
		bgptemplate.Setup,
		bgpvpn.Setup,
		filterrule.Setup,
		igmpproxyinterface.Setup,
		ospfarea.Setup,
		ospfarearange.Setup,
		ospfinstance.Setup,
		ospfinterfacetemplate.Setup,
		rule.Setup,
		table.Setup,
		community.Setup,
		certificate.Setup,
		certificatescepserver.Setup,
		clock.Setup,
		identitysystem.Setup,
		led.Setup,
		ledsettings.Setup,
		logging.Setup,
		loggingaction.Setup,
		note.Setup,
		ntpclient.Setup,
		ntpserver.Setup,
		routerboardbuttonmode.Setup,
		routerboardbuttonreset.Setup,
		routerboardbuttonwps.Setup,
		routerboardsettings.Setup,
		routerboardusb.Setup,
		schedulersystem.Setup,
		script.Setup,
		user.Setup,
		useraaa.Setup,
		usergroup.Setup,
		usersettings.Setup,
		usersshkeys.Setup,
		bandwidthserver.Setup,
		email.Setup,
		graphinginterface.Setup,
		graphingqueue.Setup,
		graphingresource.Setup,
		macserver.Setup,
		macserverping.Setup,
		macserverwinbox.Setup,
		netwatch.Setup,
		sniffer.Setup,
		manageradvanced.Setup,
		managerattribute.Setup,
		managerdatabase.Setup,
		managerlimitation.Setup,
		managerprofile.Setup,
		managerprofilelimitation.Setup,
		managerrouter.Setup,
		managersettings.Setup,
		manageruser.Setup,
		managerusergroup.Setup,
		manageruserprofile.Setup,
		aaawifi.Setup,
		accesslistwifi.Setup,
		cap.Setup,
		capsman.Setup,
		channelwifi.Setup,
		configurationwifi.Setup,
		datapathwifi.Setup,
		interworking.Setup,
		provisioningwifi.Setup,
		securitywifi.Setup,
		securitymultipassphrase.Setup,
		steering.Setup,
		keys.Setup,
		peer.Setup,
		zerotiercontroller.Setup,
		zerotierinterface.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupGated creates all controllers with the supplied logger and adds them to
// the supplied manager gated.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		mlag.SetupGated,
		port.SetupGated,
		vlan.SetupGated,
		aaa.SetupGated,
		accesslist.SetupGated,
		capsmaninterface.SetupGated,
		channel.SetupGated,
		configuration.SetupGated,
		datapath.SetupGated,
		manager.SetupGated,
		managerinterface.SetupGated,
		provisioning.SetupGated,
		rates.SetupGated,
		security.SetupGated,
		scepserver.SetupGated,
		config.SetupGated,
		envs.SetupGated,
		mounts.SetupGated,
		client.SetupGated,
		clientoption.SetupGated,
		server.SetupGated,
		serverlease.SetupGated,
		servernetwork.SetupGated,
		settings.SetupGated,
		record.SetupGated,
		addrlist.SetupGated,
		filter.SetupGated,
		mangle.SetupGated,
		nat.SetupGated,
		bonding.SetupGated,
		bridge.SetupGated,
		bridgefilter.SetupGated,
		bridgeport.SetupGated,
		bridgesettings.SetupGated,
		bridgevlan.SetupGated,
		detectinternet.SetupGated,
		dot1xclient.SetupGated,
		dot1xserver.SetupGated,
		eoip.SetupGated,
		ethernet.SetupGated,
		ethernetswitch.SetupGated,
		ethernetswitchcrs.SetupGated,
		ethernetswitchcrsegressvlantag.SetupGated,
		ethernetswitchcrsegressvlantranslation.SetupGated,
		ethernetswitchcrsingressvlantranslation.SetupGated,
		ethernetswitchcrsvlan.SetupGated,
		ethernetswitchhost.SetupGated,
		ethernetswitchport.SetupGated,
		ethernetswitchportisolation.SetupGated,
		ethernetswitchrule.SetupGated,
		ethernetswitchvlan.SetupGated,
		gre.SetupGated,
		gre6.SetupGated,
		ipip.SetupGated,
		l2tpclient.SetupGated,
		list.SetupGated,
		listmember.SetupGated,
		lte.SetupGated,
		lteapn.SetupGated,
		macvlan.SetupGated,
		ovpnclient.SetupGated,
		ovpnserver.SetupGated,
		pppoeclient.SetupGated,
		pppoeserver.SetupGated,
		sixtofour.SetupGated,
		sstpclient.SetupGated,
		sstpserver.SetupGated,
		veth.SetupGated,
		vlaninterface.SetupGated,
		vrrp.SetupGated,
		vxlan.SetupGated,
		vxlanvteps.SetupGated,
		w60g.SetupGated,
		w60gstation.SetupGated,
		wireguard.SetupGated,
		wireguardpeer.SetupGated,
		wireless.SetupGated,
		wirelessaccesslist.SetupGated,
		wirelesscap.SetupGated,
		wirelessconnectlist.SetupGated,
		wirelesssecurityprofiles.SetupGated,
		address.SetupGated,
		cloud.SetupGated,
		cloudadvanced.SetupGated,
		dhcpclient.SetupGated,
		dhcpclientoption.SetupGated,
		dhcprelay.SetupGated,
		dhcpserver.SetupGated,
		dhcpserverconfig.SetupGated,
		dhcpserverlease.SetupGated,
		dhcpservernetwork.SetupGated,
		dhcpserveroption.SetupGated,
		dhcpserveroptionmatcher.SetupGated,
		dhcpserveroptionset.SetupGated,
		dhcpserveroptionsets.SetupGated,
		dns.SetupGated,
		dnsadlist.SetupGated,
		dnsforwarders.SetupGated,
		dnsrecord.SetupGated,
		firewalladdrlist.SetupGated,
		firewallconnectiontracking.SetupGated,
		firewallfilter.SetupGated,
		firewalllayer7protocol.SetupGated,
		firewallmangle.SetupGated,
		firewallnat.SetupGated,
		firewallraw.SetupGated,
		hotspot.SetupGated,
		hotspotipbinding.SetupGated,
		hotspotprofile.SetupGated,
		hotspotserviceport.SetupGated,
		hotspotuser.SetupGated,
		hotspotuserprofile.SetupGated,
		hotspotwalledgarden.SetupGated,
		hotspotwalledgardenip.SetupGated,
		ipsecidentity.SetupGated,
		ipseckey.SetupGated,
		ipsecmodeconfig.SetupGated,
		ipsecpeer.SetupGated,
		ipsecpolicy.SetupGated,
		ipsecpolicygroup.SetupGated,
		ipsecprofile.SetupGated,
		ipsecproposal.SetupGated,
		ipsecsettings.SetupGated,
		natpmp.SetupGated,
		natpmpinterfaces.SetupGated,
		neighbordiscoverysettings.SetupGated,
		pool.SetupGated,
		route.SetupGated,
		service.SetupGated,
		settingsip.SetupGated,
		smb.SetupGated,
		sshserver.SetupGated,
		tftp.SetupGated,
		tftpsettings.SetupGated,
		trafficflow.SetupGated,
		trafficflowipfix.SetupGated,
		trafficflowtarget.SetupGated,
		upnp.SetupGated,
		upnpinterfaces.SetupGated,
		vrf.SetupGated,
		addressipv6.SetupGated,
		dhcpclientipv6.SetupGated,
		dhcpclientoptionipv6.SetupGated,
		dhcpserveripv6.SetupGated,
		dhcpserveroptionipv6.SetupGated,
		dhcpserveroptionsetsipv6.SetupGated,
		firewalladdrlistipv6.SetupGated,
		firewallfilteripv6.SetupGated,
		firewallmangleipv6.SetupGated,
		firewallnatipv6.SetupGated,
		ndprefix.SetupGated,
		neighbordiscovery.SetupGated,
		poolipv6.SetupGated,
		routeipv6.SetupGated,
		settingsipv6.SetupGated,
		items.SetupGated,
		serverovpn.SetupGated,
		aaappp.SetupGated,
		profile.SetupGated,
		secret.SetupGated,
		providerconfig.SetupGated,
		queuetype.SetupGated,
		simple.SetupGated,
		tree.SetupGated,
		incoming.SetupGated,
		bridgerouteros.SetupGated,
		container.SetupGated,
		dnsrouteros.SetupGated,
		file.SetupGated,
		grerouteros.SetupGated,
		identity.SetupGated,
		ipiprouteros.SetupGated,
		radius.SetupGated,
		scheduler.SetupGated,
		snmp.SetupGated,
		vlanrouteros.SetupGated,
		vrrprouteros.SetupGated,
		wifi.SetupGated,
		wireguardrouteros.SetupGated,
		zerotier.SetupGated,
		bfdconfiguration.SetupGated,
		bgpconnection.SetupGated,
		bgpevpn.SetupGated,
		bgpinstance.SetupGated,
		bgptemplate.SetupGated,
		bgpvpn.SetupGated,
		filterrule.SetupGated,
		igmpproxyinterface.SetupGated,
		ospfarea.SetupGated,
		ospfarearange.SetupGated,
		ospfinstance.SetupGated,
		ospfinterfacetemplate.SetupGated,
		rule.SetupGated,
		table.SetupGated,
		community.SetupGated,
		certificate.SetupGated,
		certificatescepserver.SetupGated,
		clock.SetupGated,
		identitysystem.SetupGated,
		led.SetupGated,
		ledsettings.SetupGated,
		logging.SetupGated,
		loggingaction.SetupGated,
		note.SetupGated,
		ntpclient.SetupGated,
		ntpserver.SetupGated,
		routerboardbuttonmode.SetupGated,
		routerboardbuttonreset.SetupGated,
		routerboardbuttonwps.SetupGated,
		routerboardsettings.SetupGated,
		routerboardusb.SetupGated,
		schedulersystem.SetupGated,
		script.SetupGated,
		user.SetupGated,
		useraaa.SetupGated,
		usergroup.SetupGated,
		usersettings.SetupGated,
		usersshkeys.SetupGated,
		bandwidthserver.SetupGated,
		email.SetupGated,
		graphinginterface.SetupGated,
		graphingqueue.SetupGated,
		graphingresource.SetupGated,
		macserver.SetupGated,
		macserverping.SetupGated,
		macserverwinbox.SetupGated,
		netwatch.SetupGated,
		sniffer.SetupGated,
		manageradvanced.SetupGated,
		managerattribute.SetupGated,
		managerdatabase.SetupGated,
		managerlimitation.SetupGated,
		managerprofile.SetupGated,
		managerprofilelimitation.SetupGated,
		managerrouter.SetupGated,
		managersettings.SetupGated,
		manageruser.SetupGated,
		managerusergroup.SetupGated,
		manageruserprofile.SetupGated,
		aaawifi.SetupGated,
		accesslistwifi.SetupGated,
		cap.SetupGated,
		capsman.SetupGated,
		channelwifi.SetupGated,
		configurationwifi.SetupGated,
		datapathwifi.SetupGated,
		interworking.SetupGated,
		provisioningwifi.SetupGated,
		securitywifi.SetupGated,
		securitymultipassphrase.SetupGated,
		steering.SetupGated,
		keys.SetupGated,
		peer.SetupGated,
		zerotiercontroller.SetupGated,
		zerotierinterface.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupWebhookWithManager registers conversion webhooks for all resource kinds in the group.
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	for _, setup := range []func(ctrl.Manager) error{
		mlag.SetupWebhookWithManager,
		port.SetupWebhookWithManager,
		vlan.SetupWebhookWithManager,
		aaa.SetupWebhookWithManager,
		accesslist.SetupWebhookWithManager,
		capsmaninterface.SetupWebhookWithManager,
		channel.SetupWebhookWithManager,
		configuration.SetupWebhookWithManager,
		datapath.SetupWebhookWithManager,
		manager.SetupWebhookWithManager,
		managerinterface.SetupWebhookWithManager,
		provisioning.SetupWebhookWithManager,
		rates.SetupWebhookWithManager,
		security.SetupWebhookWithManager,
		scepserver.SetupWebhookWithManager,
		config.SetupWebhookWithManager,
		envs.SetupWebhookWithManager,
		mounts.SetupWebhookWithManager,
		client.SetupWebhookWithManager,
		clientoption.SetupWebhookWithManager,
		server.SetupWebhookWithManager,
		serverlease.SetupWebhookWithManager,
		servernetwork.SetupWebhookWithManager,
		settings.SetupWebhookWithManager,
		record.SetupWebhookWithManager,
		addrlist.SetupWebhookWithManager,
		filter.SetupWebhookWithManager,
		mangle.SetupWebhookWithManager,
		nat.SetupWebhookWithManager,
		bonding.SetupWebhookWithManager,
		bridge.SetupWebhookWithManager,
		bridgefilter.SetupWebhookWithManager,
		bridgeport.SetupWebhookWithManager,
		bridgesettings.SetupWebhookWithManager,
		bridgevlan.SetupWebhookWithManager,
		detectinternet.SetupWebhookWithManager,
		dot1xclient.SetupWebhookWithManager,
		dot1xserver.SetupWebhookWithManager,
		eoip.SetupWebhookWithManager,
		ethernet.SetupWebhookWithManager,
		ethernetswitch.SetupWebhookWithManager,
		ethernetswitchcrs.SetupWebhookWithManager,
		ethernetswitchcrsegressvlantag.SetupWebhookWithManager,
		ethernetswitchcrsegressvlantranslation.SetupWebhookWithManager,
		ethernetswitchcrsingressvlantranslation.SetupWebhookWithManager,
		ethernetswitchcrsvlan.SetupWebhookWithManager,
		ethernetswitchhost.SetupWebhookWithManager,
		ethernetswitchport.SetupWebhookWithManager,
		ethernetswitchportisolation.SetupWebhookWithManager,
		ethernetswitchrule.SetupWebhookWithManager,
		ethernetswitchvlan.SetupWebhookWithManager,
		gre.SetupWebhookWithManager,
		gre6.SetupWebhookWithManager,
		ipip.SetupWebhookWithManager,
		l2tpclient.SetupWebhookWithManager,
		list.SetupWebhookWithManager,
		listmember.SetupWebhookWithManager,
		lte.SetupWebhookWithManager,
		lteapn.SetupWebhookWithManager,
		macvlan.SetupWebhookWithManager,
		ovpnclient.SetupWebhookWithManager,
		ovpnserver.SetupWebhookWithManager,
		pppoeclient.SetupWebhookWithManager,
		pppoeserver.SetupWebhookWithManager,
		sixtofour.SetupWebhookWithManager,
		sstpclient.SetupWebhookWithManager,
		sstpserver.SetupWebhookWithManager,
		veth.SetupWebhookWithManager,
		vlaninterface.SetupWebhookWithManager,
		vrrp.SetupWebhookWithManager,
		vxlan.SetupWebhookWithManager,
		vxlanvteps.SetupWebhookWithManager,
		w60g.SetupWebhookWithManager,
		w60gstation.SetupWebhookWithManager,
		wireguard.SetupWebhookWithManager,
		wireguardpeer.SetupWebhookWithManager,
		wireless.SetupWebhookWithManager,
		wirelessaccesslist.SetupWebhookWithManager,
		wirelesscap.SetupWebhookWithManager,
		wirelessconnectlist.SetupWebhookWithManager,
		wirelesssecurityprofiles.SetupWebhookWithManager,
		address.SetupWebhookWithManager,
		cloud.SetupWebhookWithManager,
		cloudadvanced.SetupWebhookWithManager,
		dhcpclient.SetupWebhookWithManager,
		dhcpclientoption.SetupWebhookWithManager,
		dhcprelay.SetupWebhookWithManager,
		dhcpserver.SetupWebhookWithManager,
		dhcpserverconfig.SetupWebhookWithManager,
		dhcpserverlease.SetupWebhookWithManager,
		dhcpservernetwork.SetupWebhookWithManager,
		dhcpserveroption.SetupWebhookWithManager,
		dhcpserveroptionmatcher.SetupWebhookWithManager,
		dhcpserveroptionset.SetupWebhookWithManager,
		dhcpserveroptionsets.SetupWebhookWithManager,
		dns.SetupWebhookWithManager,
		dnsadlist.SetupWebhookWithManager,
		dnsforwarders.SetupWebhookWithManager,
		dnsrecord.SetupWebhookWithManager,
		firewalladdrlist.SetupWebhookWithManager,
		firewallconnectiontracking.SetupWebhookWithManager,
		firewallfilter.SetupWebhookWithManager,
		firewalllayer7protocol.SetupWebhookWithManager,
		firewallmangle.SetupWebhookWithManager,
		firewallnat.SetupWebhookWithManager,
		firewallraw.SetupWebhookWithManager,
		hotspot.SetupWebhookWithManager,
		hotspotipbinding.SetupWebhookWithManager,
		hotspotprofile.SetupWebhookWithManager,
		hotspotserviceport.SetupWebhookWithManager,
		hotspotuser.SetupWebhookWithManager,
		hotspotuserprofile.SetupWebhookWithManager,
		hotspotwalledgarden.SetupWebhookWithManager,
		hotspotwalledgardenip.SetupWebhookWithManager,
		ipsecidentity.SetupWebhookWithManager,
		ipseckey.SetupWebhookWithManager,
		ipsecmodeconfig.SetupWebhookWithManager,
		ipsecpeer.SetupWebhookWithManager,
		ipsecpolicy.SetupWebhookWithManager,
		ipsecpolicygroup.SetupWebhookWithManager,
		ipsecprofile.SetupWebhookWithManager,
		ipsecproposal.SetupWebhookWithManager,
		ipsecsettings.SetupWebhookWithManager,
		natpmp.SetupWebhookWithManager,
		natpmpinterfaces.SetupWebhookWithManager,
		neighbordiscoverysettings.SetupWebhookWithManager,
		pool.SetupWebhookWithManager,
		route.SetupWebhookWithManager,
		service.SetupWebhookWithManager,
		settingsip.SetupWebhookWithManager,
		smb.SetupWebhookWithManager,
		sshserver.SetupWebhookWithManager,
		tftp.SetupWebhookWithManager,
		tftpsettings.SetupWebhookWithManager,
		trafficflow.SetupWebhookWithManager,
		trafficflowipfix.SetupWebhookWithManager,
		trafficflowtarget.SetupWebhookWithManager,
		upnp.SetupWebhookWithManager,
		upnpinterfaces.SetupWebhookWithManager,
		vrf.SetupWebhookWithManager,
		addressipv6.SetupWebhookWithManager,
		dhcpclientipv6.SetupWebhookWithManager,
		dhcpclientoptionipv6.SetupWebhookWithManager,
		dhcpserveripv6.SetupWebhookWithManager,
		dhcpserveroptionipv6.SetupWebhookWithManager,
		dhcpserveroptionsetsipv6.SetupWebhookWithManager,
		firewalladdrlistipv6.SetupWebhookWithManager,
		firewallfilteripv6.SetupWebhookWithManager,
		firewallmangleipv6.SetupWebhookWithManager,
		firewallnatipv6.SetupWebhookWithManager,
		ndprefix.SetupWebhookWithManager,
		neighbordiscovery.SetupWebhookWithManager,
		poolipv6.SetupWebhookWithManager,
		routeipv6.SetupWebhookWithManager,
		settingsipv6.SetupWebhookWithManager,
		items.SetupWebhookWithManager,
		serverovpn.SetupWebhookWithManager,
		aaappp.SetupWebhookWithManager,
		profile.SetupWebhookWithManager,
		secret.SetupWebhookWithManager,
		providerconfig.SetupWebhookWithManager,
		queuetype.SetupWebhookWithManager,
		simple.SetupWebhookWithManager,
		tree.SetupWebhookWithManager,
		incoming.SetupWebhookWithManager,
		bridgerouteros.SetupWebhookWithManager,
		container.SetupWebhookWithManager,
		dnsrouteros.SetupWebhookWithManager,
		file.SetupWebhookWithManager,
		grerouteros.SetupWebhookWithManager,
		identity.SetupWebhookWithManager,
		ipiprouteros.SetupWebhookWithManager,
		radius.SetupWebhookWithManager,
		scheduler.SetupWebhookWithManager,
		snmp.SetupWebhookWithManager,
		vlanrouteros.SetupWebhookWithManager,
		vrrprouteros.SetupWebhookWithManager,
		wifi.SetupWebhookWithManager,
		wireguardrouteros.SetupWebhookWithManager,
		zerotier.SetupWebhookWithManager,
		bfdconfiguration.SetupWebhookWithManager,
		bgpconnection.SetupWebhookWithManager,
		bgpevpn.SetupWebhookWithManager,
		bgpinstance.SetupWebhookWithManager,
		bgptemplate.SetupWebhookWithManager,
		bgpvpn.SetupWebhookWithManager,
		filterrule.SetupWebhookWithManager,
		igmpproxyinterface.SetupWebhookWithManager,
		ospfarea.SetupWebhookWithManager,
		ospfarearange.SetupWebhookWithManager,
		ospfinstance.SetupWebhookWithManager,
		ospfinterfacetemplate.SetupWebhookWithManager,
		rule.SetupWebhookWithManager,
		table.SetupWebhookWithManager,
		community.SetupWebhookWithManager,
		certificate.SetupWebhookWithManager,
		certificatescepserver.SetupWebhookWithManager,
		clock.SetupWebhookWithManager,
		identitysystem.SetupWebhookWithManager,
		led.SetupWebhookWithManager,
		ledsettings.SetupWebhookWithManager,
		logging.SetupWebhookWithManager,
		loggingaction.SetupWebhookWithManager,
		note.SetupWebhookWithManager,
		ntpclient.SetupWebhookWithManager,
		ntpserver.SetupWebhookWithManager,
		routerboardbuttonmode.SetupWebhookWithManager,
		routerboardbuttonreset.SetupWebhookWithManager,
		routerboardbuttonwps.SetupWebhookWithManager,
		routerboardsettings.SetupWebhookWithManager,
		routerboardusb.SetupWebhookWithManager,
		schedulersystem.SetupWebhookWithManager,
		script.SetupWebhookWithManager,
		user.SetupWebhookWithManager,
		useraaa.SetupWebhookWithManager,
		usergroup.SetupWebhookWithManager,
		usersettings.SetupWebhookWithManager,
		usersshkeys.SetupWebhookWithManager,
		bandwidthserver.SetupWebhookWithManager,
		email.SetupWebhookWithManager,
		graphinginterface.SetupWebhookWithManager,
		graphingqueue.SetupWebhookWithManager,
		graphingresource.SetupWebhookWithManager,
		macserver.SetupWebhookWithManager,
		macserverping.SetupWebhookWithManager,
		macserverwinbox.SetupWebhookWithManager,
		netwatch.SetupWebhookWithManager,
		sniffer.SetupWebhookWithManager,
		manageradvanced.SetupWebhookWithManager,
		managerattribute.SetupWebhookWithManager,
		managerdatabase.SetupWebhookWithManager,
		managerlimitation.SetupWebhookWithManager,
		managerprofile.SetupWebhookWithManager,
		managerprofilelimitation.SetupWebhookWithManager,
		managerrouter.SetupWebhookWithManager,
		managersettings.SetupWebhookWithManager,
		manageruser.SetupWebhookWithManager,
		managerusergroup.SetupWebhookWithManager,
		manageruserprofile.SetupWebhookWithManager,
		aaawifi.SetupWebhookWithManager,
		accesslistwifi.SetupWebhookWithManager,
		cap.SetupWebhookWithManager,
		capsman.SetupWebhookWithManager,
		channelwifi.SetupWebhookWithManager,
		configurationwifi.SetupWebhookWithManager,
		datapathwifi.SetupWebhookWithManager,
		interworking.SetupWebhookWithManager,
		provisioningwifi.SetupWebhookWithManager,
		securitywifi.SetupWebhookWithManager,
		securitymultipassphrase.SetupWebhookWithManager,
		steering.SetupWebhookWithManager,
		keys.SetupWebhookWithManager,
		peer.SetupWebhookWithManager,
		zerotiercontroller.SetupWebhookWithManager,
		zerotierinterface.SetupWebhookWithManager,
	} {
		if err := setup(mgr); err != nil {
			return err
		}
	}
	return nil
}
