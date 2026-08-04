// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/upjet/v2/pkg/controller"

	mlag "github.com/sindrip/provider-routeros/internal/controller/cluster/bridge/mlag"
	port "github.com/sindrip/provider-routeros/internal/controller/cluster/bridge/port"
	vlan "github.com/sindrip/provider-routeros/internal/controller/cluster/bridge/vlan"
	aaa "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/aaa"
	accesslist "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/accesslist"
	capsmaninterface "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/capsmaninterface"
	channel "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/channel"
	configuration "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/configuration"
	datapath "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/datapath"
	manager "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/manager"
	managerinterface "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/managerinterface"
	provisioning "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/provisioning"
	rates "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/rates"
	security "github.com/sindrip/provider-routeros/internal/controller/cluster/capsman/security"
	scepserver "github.com/sindrip/provider-routeros/internal/controller/cluster/certificate/scepserver"
	config "github.com/sindrip/provider-routeros/internal/controller/cluster/container/config"
	envs "github.com/sindrip/provider-routeros/internal/controller/cluster/container/envs"
	mounts "github.com/sindrip/provider-routeros/internal/controller/cluster/container/mounts"
	client "github.com/sindrip/provider-routeros/internal/controller/cluster/dhcp/client"
	clientoption "github.com/sindrip/provider-routeros/internal/controller/cluster/dhcp/clientoption"
	server "github.com/sindrip/provider-routeros/internal/controller/cluster/dhcp/server"
	serverlease "github.com/sindrip/provider-routeros/internal/controller/cluster/dhcp/serverlease"
	servernetwork "github.com/sindrip/provider-routeros/internal/controller/cluster/dhcp/servernetwork"
	settings "github.com/sindrip/provider-routeros/internal/controller/cluster/disk/settings"
	record "github.com/sindrip/provider-routeros/internal/controller/cluster/dns/record"
	addrlist "github.com/sindrip/provider-routeros/internal/controller/cluster/firewall/addrlist"
	filter "github.com/sindrip/provider-routeros/internal/controller/cluster/firewall/filter"
	mangle "github.com/sindrip/provider-routeros/internal/controller/cluster/firewall/mangle"
	nat "github.com/sindrip/provider-routeros/internal/controller/cluster/firewall/nat"
	bonding "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/bonding"
	bridge "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/bridge"
	bridgefilter "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/bridgefilter"
	bridgeport "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/bridgeport"
	bridgesettings "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/bridgesettings"
	bridgevlan "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/bridgevlan"
	detectinternet "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/detectinternet"
	dot1xclient "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/dot1xclient"
	dot1xserver "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/dot1xserver"
	eoip "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/eoip"
	ethernet "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernet"
	ethernetswitch "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitch"
	ethernetswitchcrs "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchcrs"
	ethernetswitchcrsegressvlantag "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchcrsegressvlantag"
	ethernetswitchcrsegressvlantranslation "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchcrsegressvlantranslation"
	ethernetswitchcrsingressvlantranslation "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchcrsingressvlantranslation"
	ethernetswitchcrsvlan "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchcrsvlan"
	ethernetswitchhost "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchhost"
	ethernetswitchport "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchport"
	ethernetswitchportisolation "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchportisolation"
	ethernetswitchrule "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchrule"
	ethernetswitchvlan "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ethernetswitchvlan"
	gre "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/gre"
	gre6 "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/gre6"
	ipip "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ipip"
	l2tpclient "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/l2tpclient"
	list "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/list"
	listmember "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/listmember"
	lte "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/lte"
	lteapn "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/lteapn"
	macvlan "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/macvlan"
	ovpnclient "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ovpnclient"
	ovpnserver "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/ovpnserver"
	pppoeclient "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/pppoeclient"
	pppoeserver "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/pppoeserver"
	sixtofour "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/sixtofour"
	sstpclient "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/sstpclient"
	sstpserver "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/sstpserver"
	veth "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/veth"
	vlaninterface "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/vlan"
	vrrp "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/vrrp"
	vxlan "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/vxlan"
	vxlanvteps "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/vxlanvteps"
	w60g "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/w60g"
	w60gstation "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/w60gstation"
	wireguard "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/wireguard"
	wireguardpeer "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/wireguardpeer"
	wireless "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/wireless"
	wirelessaccesslist "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/wirelessaccesslist"
	wirelesscap "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/wirelesscap"
	wirelessconnectlist "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/wirelessconnectlist"
	wirelesssecurityprofiles "github.com/sindrip/provider-routeros/internal/controller/cluster/interface/wirelesssecurityprofiles"
	address "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/address"
	cloud "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/cloud"
	cloudadvanced "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/cloudadvanced"
	dhcpclient "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpclient"
	dhcpclientoption "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpclientoption"
	dhcprelay "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcprelay"
	dhcpserver "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpserver"
	dhcpserverconfig "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpserverconfig"
	dhcpserverlease "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpserverlease"
	dhcpservernetwork "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpservernetwork"
	dhcpserveroption "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpserveroption"
	dhcpserveroptionmatcher "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpserveroptionmatcher"
	dhcpserveroptionset "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpserveroptionset"
	dhcpserveroptionsets "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dhcpserveroptionsets"
	dns "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dns"
	dnsadlist "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dnsadlist"
	dnsforwarders "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dnsforwarders"
	dnsrecord "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/dnsrecord"
	firewalladdrlist "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/firewalladdrlist"
	firewallconnectiontracking "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/firewallconnectiontracking"
	firewallfilter "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/firewallfilter"
	firewalllayer7protocol "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/firewalllayer7protocol"
	firewallmangle "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/firewallmangle"
	firewallnat "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/firewallnat"
	firewallraw "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/firewallraw"
	hotspot "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/hotspot"
	hotspotipbinding "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/hotspotipbinding"
	hotspotprofile "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/hotspotprofile"
	hotspotserviceport "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/hotspotserviceport"
	hotspotuser "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/hotspotuser"
	hotspotuserprofile "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/hotspotuserprofile"
	hotspotwalledgarden "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/hotspotwalledgarden"
	hotspotwalledgardenip "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/hotspotwalledgardenip"
	ipsecidentity "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipsecidentity"
	ipseckey "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipseckey"
	ipsecmodeconfig "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipsecmodeconfig"
	ipsecpeer "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipsecpeer"
	ipsecpolicy "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipsecpolicy"
	ipsecpolicygroup "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipsecpolicygroup"
	ipsecprofile "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipsecprofile"
	ipsecproposal "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipsecproposal"
	ipsecsettings "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/ipsecsettings"
	natpmp "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/natpmp"
	natpmpinterfaces "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/natpmpinterfaces"
	neighbordiscoverysettings "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/neighbordiscoverysettings"
	pool "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/pool"
	route "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/route"
	service "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/service"
	settingsip "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/settings"
	smb "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/smb"
	sshserver "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/sshserver"
	tftp "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/tftp"
	tftpsettings "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/tftpsettings"
	trafficflow "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/trafficflow"
	trafficflowipfix "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/trafficflowipfix"
	trafficflowtarget "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/trafficflowtarget"
	upnp "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/upnp"
	upnpinterfaces "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/upnpinterfaces"
	vrf "github.com/sindrip/provider-routeros/internal/controller/cluster/ip/vrf"
	addressipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/address"
	dhcpclientipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/dhcpclient"
	dhcpclientoptionipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/dhcpclientoption"
	dhcpserveripv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/dhcpserver"
	dhcpserveroptionipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/dhcpserveroption"
	dhcpserveroptionsetsipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/dhcpserveroptionsets"
	firewalladdrlistipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/firewalladdrlist"
	firewallfilteripv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/firewallfilter"
	firewallmangleipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/firewallmangle"
	firewallnatipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/firewallnat"
	ndprefix "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/ndprefix"
	neighbordiscovery "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/neighbordiscovery"
	poolipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/pool"
	routeipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/route"
	settingsipv6 "github.com/sindrip/provider-routeros/internal/controller/cluster/ipv6/settings"
	items "github.com/sindrip/provider-routeros/internal/controller/cluster/move/items"
	serverovpn "github.com/sindrip/provider-routeros/internal/controller/cluster/ovpn/server"
	aaappp "github.com/sindrip/provider-routeros/internal/controller/cluster/ppp/aaa"
	profile "github.com/sindrip/provider-routeros/internal/controller/cluster/ppp/profile"
	secret "github.com/sindrip/provider-routeros/internal/controller/cluster/ppp/secret"
	providerconfig "github.com/sindrip/provider-routeros/internal/controller/cluster/providerconfig"
	queuetype "github.com/sindrip/provider-routeros/internal/controller/cluster/queue/queuetype"
	simple "github.com/sindrip/provider-routeros/internal/controller/cluster/queue/simple"
	tree "github.com/sindrip/provider-routeros/internal/controller/cluster/queue/tree"
	incoming "github.com/sindrip/provider-routeros/internal/controller/cluster/radius/incoming"
	bridgerouteros "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/bridge"
	container "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/container"
	dnsrouteros "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/dns"
	file "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/file"
	grerouteros "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/gre"
	identity "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/identity"
	ipiprouteros "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/ipip"
	radius "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/radius"
	scheduler "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/scheduler"
	snmp "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/snmp"
	vlanrouteros "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/vlan"
	vrrprouteros "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/vrrp"
	wifi "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/wifi"
	wireguardrouteros "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/wireguard"
	zerotier "github.com/sindrip/provider-routeros/internal/controller/cluster/routeros/zerotier"
	bfdconfiguration "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/bfdconfiguration"
	bgpconnection "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/bgpconnection"
	bgpevpn "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/bgpevpn"
	bgpinstance "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/bgpinstance"
	bgptemplate "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/bgptemplate"
	bgpvpn "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/bgpvpn"
	filterrule "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/filterrule"
	igmpproxyinterface "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/igmpproxyinterface"
	ospfarea "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/ospfarea"
	ospfarearange "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/ospfarearange"
	ospfinstance "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/ospfinstance"
	ospfinterfacetemplate "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/ospfinterfacetemplate"
	rule "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/rule"
	table "github.com/sindrip/provider-routeros/internal/controller/cluster/routing/table"
	community "github.com/sindrip/provider-routeros/internal/controller/cluster/snmp/community"
	certificate "github.com/sindrip/provider-routeros/internal/controller/cluster/system/certificate"
	certificatescepserver "github.com/sindrip/provider-routeros/internal/controller/cluster/system/certificatescepserver"
	clock "github.com/sindrip/provider-routeros/internal/controller/cluster/system/clock"
	identitysystem "github.com/sindrip/provider-routeros/internal/controller/cluster/system/identity"
	led "github.com/sindrip/provider-routeros/internal/controller/cluster/system/led"
	ledsettings "github.com/sindrip/provider-routeros/internal/controller/cluster/system/ledsettings"
	logging "github.com/sindrip/provider-routeros/internal/controller/cluster/system/logging"
	loggingaction "github.com/sindrip/provider-routeros/internal/controller/cluster/system/loggingaction"
	note "github.com/sindrip/provider-routeros/internal/controller/cluster/system/note"
	ntpclient "github.com/sindrip/provider-routeros/internal/controller/cluster/system/ntpclient"
	ntpserver "github.com/sindrip/provider-routeros/internal/controller/cluster/system/ntpserver"
	routerboardbuttonmode "github.com/sindrip/provider-routeros/internal/controller/cluster/system/routerboardbuttonmode"
	routerboardbuttonreset "github.com/sindrip/provider-routeros/internal/controller/cluster/system/routerboardbuttonreset"
	routerboardbuttonwps "github.com/sindrip/provider-routeros/internal/controller/cluster/system/routerboardbuttonwps"
	routerboardsettings "github.com/sindrip/provider-routeros/internal/controller/cluster/system/routerboardsettings"
	routerboardusb "github.com/sindrip/provider-routeros/internal/controller/cluster/system/routerboardusb"
	schedulersystem "github.com/sindrip/provider-routeros/internal/controller/cluster/system/scheduler"
	script "github.com/sindrip/provider-routeros/internal/controller/cluster/system/script"
	user "github.com/sindrip/provider-routeros/internal/controller/cluster/system/user"
	useraaa "github.com/sindrip/provider-routeros/internal/controller/cluster/system/useraaa"
	usergroup "github.com/sindrip/provider-routeros/internal/controller/cluster/system/usergroup"
	usersettings "github.com/sindrip/provider-routeros/internal/controller/cluster/system/usersettings"
	usersshkeys "github.com/sindrip/provider-routeros/internal/controller/cluster/system/usersshkeys"
	bandwidthserver "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/bandwidthserver"
	email "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/email"
	graphinginterface "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/graphinginterface"
	graphingqueue "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/graphingqueue"
	graphingresource "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/graphingresource"
	macserver "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/macserver"
	macserverping "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/macserverping"
	macserverwinbox "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/macserverwinbox"
	netwatch "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/netwatch"
	sniffer "github.com/sindrip/provider-routeros/internal/controller/cluster/tool/sniffer"
	manageradvanced "github.com/sindrip/provider-routeros/internal/controller/cluster/user/manageradvanced"
	managerattribute "github.com/sindrip/provider-routeros/internal/controller/cluster/user/managerattribute"
	managerdatabase "github.com/sindrip/provider-routeros/internal/controller/cluster/user/managerdatabase"
	managerlimitation "github.com/sindrip/provider-routeros/internal/controller/cluster/user/managerlimitation"
	managerprofile "github.com/sindrip/provider-routeros/internal/controller/cluster/user/managerprofile"
	managerprofilelimitation "github.com/sindrip/provider-routeros/internal/controller/cluster/user/managerprofilelimitation"
	managerrouter "github.com/sindrip/provider-routeros/internal/controller/cluster/user/managerrouter"
	managersettings "github.com/sindrip/provider-routeros/internal/controller/cluster/user/managersettings"
	manageruser "github.com/sindrip/provider-routeros/internal/controller/cluster/user/manageruser"
	managerusergroup "github.com/sindrip/provider-routeros/internal/controller/cluster/user/managerusergroup"
	manageruserprofile "github.com/sindrip/provider-routeros/internal/controller/cluster/user/manageruserprofile"
	aaawifi "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/aaa"
	accesslistwifi "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/accesslist"
	cap "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/cap"
	capsman "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/capsman"
	channelwifi "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/channel"
	configurationwifi "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/configuration"
	datapathwifi "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/datapath"
	interworking "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/interworking"
	provisioningwifi "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/provisioning"
	securitywifi "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/security"
	securitymultipassphrase "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/securitymultipassphrase"
	steering "github.com/sindrip/provider-routeros/internal/controller/cluster/wifi/steering"
	keys "github.com/sindrip/provider-routeros/internal/controller/cluster/wireguard/keys"
	peer "github.com/sindrip/provider-routeros/internal/controller/cluster/wireguard/peer"
	zerotiercontroller "github.com/sindrip/provider-routeros/internal/controller/cluster/zerotier/zerotiercontroller"
	zerotierinterface "github.com/sindrip/provider-routeros/internal/controller/cluster/zerotier/zerotierinterface"
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
