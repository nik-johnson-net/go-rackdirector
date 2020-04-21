#!/bin/bash

function usage() {
    echo "Usage:"
    echo "  add <name> <id> <network-start> <domain> <description>"
    echo "  remove <name> <id> <network-start> <domain>"
    exit 1
}

function switch_add_vlan() {
    local vlanName=$1
    local vlanId=$2
    local vlanGateway=${3/%.0/.1}
    local vlanDomain=$4
    local vlanStart=${3/%.0/.2}
    local vlanStop=${3/%.0/.254}
    local vlanDescription=$5
    cat - <<EOF
configure
set vlans $vlanName vlan-id $vlanId
set vlans $vlanName description "$vlanDescription"
set vlans $vlanName l3-interface vlan.$vlanId
set firewall family inet filter ingress-vlan-$vlanName term rfc1918 from destination-address 192.168.0.0/16
set firewall family inet filter ingress-vlan-$vlanName term rfc1918 from destination-address 172.16.0.0/12
set firewall family inet filter ingress-vlan-$vlanName term rfc1918 from destination-address 10.0.0.0/8
set firewall family inet filter ingress-vlan-$vlanName term rfc1918 then reject
set firewall family inet6 filter ingress-vlan-$vlanName term private from destination-address fc00::/7
set firewall family inet6 filter ingress-vlan-$vlanName term private then reject
set interfaces vlan unit $vlanId family inet address $vlanGateway/24
set interfaces vlan unit $vlanId family inet filter input ingress-vlan-$vlanName
set interfaces vlan unit $vlanId family inet6 filter input ingress-vlan-$vlanName
commit
EOF
}

function router_add_network() {
    local vlanName=$1
    local vlanNetworkStart=$3
    local vlanDomain=$4
    local vlanStart=${3/%.0/.2}
    local vlanStop=${3/%.0/.254}
    cat - <<EOF
configure
set service dhcp-server shared-network-name $vlanName authoritative enable
set service dhcp-server shared-network-name $vlanName default-router $vlanGateway
set service dhcp-server shared-network-name $vlanName dns-server 192.168.0.1
set service dhcp-server shared-network-name $vlanName domain-name $vlanDomain
set service dhcp-server shared-network-name $vlanName start $vlanStart stop $vlanStop
set service dhcp-server shared-network-name $vlanName lease 86400
set protocols static route $vlanNetworkStart/24 next-hop 192.168.0.3
commit
EOF
}

function add() {
    switch_add_vlan "$@" | ssh -tt as-1.echo.jnstw.net 
    router_add_network "$@" | ssh -tt er-1.echo.jnstw.net 
}

function remove() {
    vlanName=$1
    vlanId=$2
    vlanNetworkStart=$3
    cat - <<EOF
configure
delete vlans $vlanName
delete firewall family inet filter ingress-vlan-$vlanName
delete firewall family inet6 filter ingress-vlan-$vlanName
delete interfaces vlan unit $vlanId
commit
EOF

    cat - <<EOF
configure
delete service dhcp-server shared-network-name $vlanName
delete protocols static route $vlanNetworkStart/24
commit
EOF
}

cmd="$1"
shift
case $cmd in
    add)
        add "$@"
        ;;
    *)
        echo "unknown command $cmd" 
        exit 1
        ;;
esac