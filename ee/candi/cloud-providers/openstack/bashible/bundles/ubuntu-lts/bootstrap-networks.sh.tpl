#!/bin/bash
{{- /*
# Copyright 2021 Flant JSC
# Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/}}
shopt -s extglob

function cat_file() {
  cat_dev=$1
  cat_metric=$2
  cat_mac=$3
  cat > /etc/netplan/100-cim-"$cat_dev".yaml <<BOOTSTRAP_NETWORK_EOF
network:
  version: 2
  ethernets:
    $cat_dev:
      dhcp4-overrides:
        route-metric: $cat_metric
      match:
        macaddress: $cat_mac
BOOTSTRAP_NETWORK_EOF
}

count_default=$(ip route show default | wc -l)
echo "count_default: $count_default"
if [[ "$count_default" -gt "1" ]]; then
  configured_macs="$(grep -Po '(?<=macaddress: ).+' /etc/netplan/50-cloud-init.yaml)"
  echo "configured_macs: $configured_macs"
  for mac in $configured_macs; do
    ifname="$(ip -o link show | grep "link/ether $mac" | cut -d ":" -f2 | tr -d " ")|"
    if [[ "$ifname" != "" ]]; then
      configured_ifnames_pattern+="$ifname "
    fi
  done
  echo "configured_ifnames_pattern: $configured_ifnames_pattern"
  count_configured_ifnames=$(echo $configured_ifnames_pattern | wc -w)
  echo "count_configured_ifnames: $count_configured_ifnames"
  if [[ "$count_configured_ifnames" -gt "1" ]]; then
    set +e
    check_metric=$(grep -Po '(?<=route-metric: ).+' /etc/netplan/50-cloud-init.yaml | wc -l)
    set -e
    if [[ "$check_metric" -eq "0" ]]; then
      metric=100
      for i in $configured_ifnames_pattern; do
        cim_dev=$i
        cim_mac="$(ip link show dev $ifname | grep "link/ether" | sed "s/  //g" | cut -d " " -f2)"
        cim_metric=$metric
        metric=$((metric + 100))
        cat_file "$cim_dev" "$cim_metric" "$cim_mac"
      done
      netplan generate
      netplan apply
    fi
  fi
fi

shopt -u extglob
