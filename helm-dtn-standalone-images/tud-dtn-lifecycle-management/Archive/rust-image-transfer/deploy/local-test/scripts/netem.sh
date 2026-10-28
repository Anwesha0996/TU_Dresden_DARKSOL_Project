#!/usr/bin/env bash
# Link disruption injection for the earth-node <-> space-node link.
#
# Both node containers run with cap_add: NET_ADMIN and ship iproute2/iptables
# (see ../ud3tn/Dockerfile) specifically so this can run against their eth0
# with no extra sidecar container needed.
#
# Usage:
#   ./netem.sh delay <node> <delay_ms> [jitter_ms] [loss_pct]
#       Add latency/jitter/loss to <node>'s link (both directions, since it
#       shapes <node>'s own egress - combined with the other side seeing the
#       same shaped traffic, this is enough to simulate a degraded DTN link).
#       Example: ./netem.sh delay space-node 500 100 5
#
#   ./netem.sh outage <node>
#       Full blackhole: drop all traffic between earth-node and space-node,
#       applied on <node> via iptables (harder cut than 100% netem loss -
#       actively refuses/drops rather than relying on qdisc probability).
#       Example: ./netem.sh outage space-node
#
#   ./netem.sh clear <node>
#       Remove both the netem qdisc and the outage iptables rules from <node>.
#       Example: ./netem.sh clear space-node
#
#   ./netem.sh status <node>
#       Show current qdisc + iptables state.
set -euo pipefail

usage() { echo "usage: $0 {delay|outage|clear|status} <earth-node|space-node> [args...]" >&2; exit 1; }

[[ $# -ge 2 ]] || usage
cmd="$1"; node="$2"; shift 2

peer() { [[ "$node" == "earth-node" ]] && echo "space-node" || echo "earth-node"; }

case "$cmd" in
    delay)
        delay_ms="${1:-500}"; jitter_ms="${2:-0}"; loss_pct="${3:-0}"
        echo "==> ${node}: netem delay ${delay_ms}ms jitter ${jitter_ms}ms loss ${loss_pct}%"
        docker exec "$node" tc qdisc replace dev eth0 root netem \
            delay "${delay_ms}ms" "${jitter_ms}ms" loss "${loss_pct}%"
        ;;
    outage)
        p="$(peer)"
        echo "==> ${node}: blackholing all traffic to/from ${p} (iptables DROP)"
        docker exec "$node" iptables -A OUTPUT -d "$p" -j DROP
        docker exec "$node" iptables -A INPUT -s "$p" -j DROP
        ;;
    clear)
        p="$(peer)"
        echo "==> ${node}: clearing netem qdisc and outage rules"
        docker exec "$node" tc qdisc del dev eth0 root 2>/dev/null || true
        docker exec "$node" iptables -D OUTPUT -d "$p" -j DROP 2>/dev/null || true
        docker exec "$node" iptables -D INPUT -s "$p" -j DROP 2>/dev/null || true
        ;;
    status)
        echo "==> ${node} qdisc:"
        docker exec "$node" tc qdisc show dev eth0
        echo "==> ${node} iptables:"
        docker exec "$node" iptables -L -v -n
        ;;
    *)
        usage
        ;;
esac
