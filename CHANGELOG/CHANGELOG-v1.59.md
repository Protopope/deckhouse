# Changelog v1.59

## [MALFORMED]


 - #5984 unknown section "matrix-tests"
 - #7244 missing section, missing summary, missing type, unknown section ""
 - #7557 unknown section "ingress-nginx, vpa"
 - #7571 missing section, missing summary, missing type, unknown section ""
 - #7632 unknown section "node-local-dns, cni-cilium"
 - #7663 unknown section "global"
 - #7684 unknown section "cilium"
 - #7719 unknown section "linstor"
 - #7759 invalid impact level "default | high | low", invalid type "fix | feature | chore", unknown section "<kebab-case of a module name> | <1st level dir in the repo>"

## Know before update


 - Deckhouse update is not possible until users migrate from the Linstor module to sds-drbd.

## Features


 - **[cloud-provider-openstack]** Add alert about orphaned disks (without PV) in the cloud. [#7458](https://github.com/deckhouse/deckhouse/pull/7458)
 - **[cloud-provider-vsphere]** Added a parameter that allows you to use an existing vm folder. [#7667](https://github.com/deckhouse/deckhouse/pull/7667)
 - **[cloud-provider-yandex]** Add a feature to discover orphaned disks for YandexCloud. [#7603](https://github.com/deckhouse/deckhouse/pull/7603)
 - **[deckhouse]** Create embedded default ModuleUpdatePolicy, if no others are set. [#7703](https://github.com/deckhouse/deckhouse/pull/7703)
 - **[deckhouse]** Add `Ignore` mode for ModuleUpdatePolicy to exclude desired modules. [#7703](https://github.com/deckhouse/deckhouse/pull/7703)
 - **[deckhouse-controller]** Provides deckhouse HA mode. [#7634](https://github.com/deckhouse/deckhouse/pull/7634)
 - **[deckhouse-controller]** Added module loading metrics. [#7424](https://github.com/deckhouse/deckhouse/pull/7424)
 - **[extended-monitoring]** Support force image check with image-availability-exporter, even when workloads are disabled or suspended. [#7606](https://github.com/deckhouse/deckhouse/pull/7606)
 - **[external-module-manager]** Disable module hooks on restart. [#7582](https://github.com/deckhouse/deckhouse/pull/7582)

## Fixes


 - **[admission-policy-engine]** Added the ability to specify restrictions on applying cluster roles to users. [#7660](https://github.com/deckhouse/deckhouse/pull/7660)
 - **[dashboard]** Add cm kube-rbac-proxy-ca.crt in namespace d8-dashboard. [#7766](https://github.com/deckhouse/deckhouse/pull/7766)
 - **[external-module-manager]** Fix panic at a module status update. [#7648](https://github.com/deckhouse/deckhouse/pull/7648)
 - **[node-manager]** Add the `PasswordAuthentication=no` option to the OpenSSH client if public key authentication is used. [#7621](https://github.com/deckhouse/deckhouse/pull/7621)
 - **[node-manager]** Check for `SSHCredentials` in the `StaticInstance` webhook. [#7619](https://github.com/deckhouse/deckhouse/pull/7619)
 - **[node-manager]** Fix the cleanup phase in the 'caps-controller-manager' if the cleanup script does not exist. [#7615](https://github.com/deckhouse/deckhouse/pull/7615)
 - **[node-manager]** Added NodeGroup validation to prevent adding more than one taint with the same key and effect. [#7474](https://github.com/deckhouse/deckhouse/pull/7474)
 - **[node-manager]** Fix a race in instance controller if NodeGroup has been deleted. [#7454](https://github.com/deckhouse/deckhouse/pull/7454)
 - **[node-manager]** added alert on impossible node drain [#6190](https://github.com/deckhouse/deckhouse/pull/6190)

## Chore


 - **[cni-cilium]** Improvement of the cilium-agent version update process [#7658](https://github.com/deckhouse/deckhouse/pull/7658)
    Cilium-agent podiums can be restarted.
 - **[deckhouse-controller]** Remove linstor module [#7091](https://github.com/deckhouse/deckhouse/pull/7091)
    Deckhouse update is not possible until users migrate from the Linstor module to sds-drbd.
 - **[prometheus]** Update metrics rotation value. [#7560](https://github.com/deckhouse/deckhouse/pull/7560)

