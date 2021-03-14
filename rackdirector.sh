#!/usr/bin/env bash

function start() {
    local hostinfo=""
    local host_ipv4=""
    local bmc_ipv4=""

    hostinfo=$(curl -s "http://10.0.1.10/api/lookup?hostname=$1")
    if [[ $? -ne 0 ]]; then
        echo "Error looking up $1:" >&2
        echo "$result" >&2
        return 1
    fi

    host_ipv4=$(echo "$hostinfo" | jq -r '.Interfaces[0] | .Ipv4')
    bmc_ipv4=$(echo "$hostinfo" | jq -r '.Bmc.Ipv4')

    echo "Starting plan $2 on $1"
    curl -s -d "{\"Address\": \"$host_ipv4\", \"Plan\": \"$2\"}" http://10.0.1.10/api/plan
    echo "Rebooting $1"
    ipmitool -Ilanplus -U ADMIN -P ADMIN -H "$bmc_ipv4" power cycle
}

function show() {
    local hostinfo=""
    local host_ipv4=""

    hostinfo=$(curl -s "http://10.0.1.10/api/lookup?hostname=$1")
    if [[ $? -ne 0 ]]; then
        echo "Error looking up $1:" >&2
        echo "$result" >&2
        return 1
    fi

    host_ipv4=$(echo "$hostinfo" | jq -r '.Interfaces[0] | .Ipv4')

    curl -s -X GET -d "{\"Address\": \"$host_ipv4\", \"Plan\": \"\"}" http://10.0.1.10/api/plan

}

case "$1" in
start) start "$2" "$3" ;;
show) show "$2" ;;
*) echo "Unknown subcommand $1" >&2; exit 1 ;;
esac
