#!/usr/bin/env bash
# N0 measurements for the privileged helper plan (docs/PLANS/cross-cutting/network-stack-privileged-additions.md, section 5, N0).
# Run on vertex as root: it starts each probe as the llm-proxy user in a TRANSIENT systemd unit that copies the service's own hardening and
# changes only the four things being measured (capabilities, address families, groups, network namespace). Nothing is installed and no
# file outside this run's output is written. Every probe first prints what is IN FORCE (/proc/self/status and one socket() per family).
#
# READ-ONLY, with two exceptions that are OFF unless you ask:
#   --simulate-neigh-change   creates and deletes a throwaway network namespace "n0probe" (no host interface or route is touched)
#   standin_listener.py       (item 5, run by hand) creates and removes /run/n0-helper-standin
# The nmap section sends packets to ONE device you name with --target (UDP to one port, a few TCP probes): a private IPv4 address that is
# yours, and it refuses anything else. The capture reads at most 128 bytes of each frame and prints counts only.
#
#   sudo ./run.sh --target 192.168.x.y [--iface wlp2s0] [--window 120] [--sections 0,1,2,3,4,5] [--simulate-neigh-change]
#   ./run.sh --print                      show the systemd-run commands only (no root needed)
# Output is also written to ./n0-results-<host>-<date>.txt with IPv4/IPv6/MAC addresses already masked. Read it before you paste it.
# A measurement run records every failure and keeps going: no -e and no pipefail (a missing log file or a probe that exits early must not end the run).
set -u

UNIT="${UNIT:-llm-proxy}"
HERE="$(cd "$(dirname "$0")" && pwd)"
TEMPLATE="$HERE/../../services/llm-proxy.service"
TARGET="" IFACE="" WINDOW=120 SECTIONS="0,1,2,3,4,5" SIMULATE=0 PRINT=0 OUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --target) TARGET="$2"; shift 2 ;;
    --iface) IFACE="$2"; shift 2 ;;
    --window) WINDOW="$2"; shift 2 ;;
    --sections) SECTIONS="$2"; shift 2 ;;
    --simulate-neigh-change) SIMULATE=1; shift ;;
    --out) OUT="$2"; shift 2 ;;
    --print) PRINT=1; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[[ "$WINDOW" =~ ^[0-9]+$ && "$WINDOW" -ge 5 && "$WINDOW" -le 600 ]] || { echo "--window must be 5..600 seconds" >&2; exit 2; }
[[ $PRINT -eq 1 || "$(id -u)" -eq 0 ]] || { echo "run as root (sudo): systemd-run needs it" >&2; exit 2; }

want() { [[ ",$SECTIONS," == *",$1,"* ]]; }
say() { printf '\n##### %s\n' "$*"; }

if [[ $PRINT -eq 0 ]]; then
  OUT="${OUT:-$PWD/n0-results-$(hostname -s)-$(date +%Y%m%d-%H%M).txt}"
  # Mask every address before it reaches the screen or the file: these results are meant for the repository.
  exec > >(sed -u -E 's/([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}/<mac>/g; s/[0-9]{1,3}(\.[0-9]{1,3}){3}(\/[0-9]+)?/<ip>/g; s/[0-9A-Fa-f]{1,4}(:[0-9A-Fa-f]{0,4}){3,7}/<ip6>/g; s/^(Nmap scan report for ).*/\1<target>/' | tee "$OUT") 2>&1
fi

# --- the service's own hardening, copied from the installed unit (or the repo template) -------------------------------------------------
if systemctl cat "$UNIT" >/dev/null 2>&1; then unit_text="$(systemctl cat "$UNIT")"; from="installed unit $UNIT"
else unit_text="$(cat "$TEMPLATE")"; from="repo template (no installed unit)"; fi
KEYS='NoNewPrivileges|ProtectSystem|ProtectHome|PrivateTmp|ProtectKernelTunables|ProtectKernelModules|ProtectKernelLogs|ProtectControlGroups|ProtectClock|ProtectHostname|RestrictNamespaces|RestrictRealtime|RestrictSUIDSGID|LockPersonality|MemoryDenyWriteExecute|SystemCallFilter|SystemCallArchitectures|SystemCallErrorNumber|User|Group|UMask'
base_props=()
while IFS= read -r line; do base_props+=(-p "$line"); done < <(printf '%s\n' "$unit_text" | grep -E "^(${KEYS})=" | sed 's/[[:space:]]*#.*$//')
unit_value() { printf '%s\n' "$unit_text" | sed -n "s/^$1=//p" | sed 's/[[:space:]]*#.*$//' | tail -1; }
FAM_UNIT="$(unit_value RestrictAddressFamilies)"; FAM_UNIT="${FAM_UNIT:-AF_UNIX AF_INET AF_INET6}"
GROUPS_UNIT="$(unit_value SupplementaryGroups)"
HARDEN_NOTE="hardening copied from the $from; the four measured properties are each set by ONE -p below (never merged with the unit's)"

# run_unit CAPS FAMILIES GROUPS [extra systemd-run args] -- command...    (the probe script is fed on stdin by the caller)
# CAPS is both the bounding and the ambient set, so a non-root user really holds them; empty means none, as in the unit.
run_unit() {
  local caps="$1" fam="$2" grp="$3"; shift 3
  local extra=()
  while [[ "$1" != "--" ]]; do extra+=("$1"); shift; done; shift
  local props=("${base_props[@]}" -p "CapabilityBoundingSet=$caps" -p "AmbientCapabilities=$caps" -p "RestrictAddressFamilies=$fam" -p "RuntimeMaxSec=$((WINDOW + 120))")
  [[ -n "$grp" ]] && props+=(-p "SupplementaryGroups=$grp")
  local cmd=(systemd-run --pipe --wait --quiet --collect "${props[@]}" ${extra[@]+"${extra[@]}"} -- "$@")
  if [[ $PRINT -eq 1 ]]; then printf '%q ' "${cmd[@]}"; echo; [[ -t 0 ]] || cat >/dev/null; return 0; fi
  "${cmd[@]}" || echo "(exit status $?)"
}
probe() { cat "$HERE/inforce.py" "$HERE/$1"; }                  # inforce.py + a probe file, as one script for python3 on stdin
in_force_only() { cat "$HERE/inforce.py"; echo 'report_in_force()'; }
variant() { # name caps families groups
  say "VARIANT $1: caps='${2:-none}' families='$3' groups='${4:-none}'"
}

default_iface() { ip -o route show default 2>/dev/null | awk '{for(i=1;i<NF;i++) if($1=="default"&&$i=="dev"){print $(i+1); exit}}' || true; }

# --- 0. facts ------------------------------------------------------------------------------------------------------------------------
if want 0 && [[ $PRINT -eq 0 ]]; then
  say "0. FACTS ($HARDEN_NOTE)"
  echo "date=$(date -u +%FT%TZ) host=$(hostname -s)"
  uname -srm; systemctl --version | head -1
  echo "lsm: $(cat /sys/kernel/security/lsm 2>/dev/null || echo unreadable)"
  echo "unit hardening as systemd sees it:"
  systemctl cat "$UNIT" >/dev/null 2>&1 || echo "(unit $UNIT is not installed here: the values below are systemd defaults, not the service)"
  systemctl show "$UNIT" -p User -p NoNewPrivileges -p CapabilityBoundingSet -p AmbientCapabilities -p RestrictAddressFamilies -p SupplementaryGroups -p SystemCallFilter 2>/dev/null | cut -c1-200 || echo "(unit not installed)"
  id llm-proxy 2>&1 | sed -E 's/\([^)]*\)//g'
  command -v nmap >/dev/null && nmap -V | head -1 || echo "nmap: not installed"
  [[ -n "$IFACE" ]] || IFACE="$(default_iface)"
  echo "interface=$IFACE wireless=$([[ -d /sys/class/net/$IFACE/wireless ]] && echo yes || echo no) ping_group_range=$(cat /proc/sys/net/ipv4/ping_group_range) yama_ptrace_scope=$(cat /proc/sys/kernel/yama/ptrace_scope 2>/dev/null || echo unreadable)"
  echo "takeover routes the helper's caller rule depends on (a same-account child must not be able to use them on the service):"
  systemd-analyze syscall-filter @system-service 2>/dev/null | grep -wE 'ptrace|pidfd_getfd|process_vm_readv|process_vm_writev' || echo "  none of ptrace, pidfd_getfd, process_vm_readv, process_vm_writev is in @system-service (the unit's filter refuses them)"
  echo "sandbox settings block (settings.yml):"
  awk '/^sandboxing:/{f=1;print;next} f&&/^[^ ]/{f=0} f' /var/lib/llm-proxy/settings.yml 2>/dev/null || echo "(settings.yml not at /var/lib/llm-proxy)"
fi
[[ -n "$IFACE" ]] || IFACE="$(default_iface)"

# --- 1. nmap -sU and -O ---------------------------------------------------------------------------------------------------------------
if want 1; then
  say "1. NMAP UDP SCAN AND OS DETECTION (one device, rate and time limited)"
  if [[ -z "$TARGET" ]]; then echo "SKIPPED: pass --target <private IPv4 of a device you own>"
  elif ! python3 -I -c 'import ipaddress,sys; a=ipaddress.ip_address(sys.argv[1]); sys.exit(0 if a.version==4 and any(a in ipaddress.ip_network(n) for n in ("10.0.0.0/8","172.16.0.0/12","192.168.0.0/16")) else 1)' "$TARGET" 2>/dev/null; then
    echo "REFUSED: $TARGET is not a private IPv4 address (10/8, 172.16/12, 192.168/16)"
  else
    NM="$(command -v nmap || true)"
    if [[ -z "$NM" && $PRINT -eq 0 ]]; then echo "SKIPPED: nmap is not installed"; NM=nmap; fi
    [[ $PRINT -eq 1 ]] && NM=nmap
    [[ $PRINT -eq 1 ]] || { echo "scanning one device for a few seconds per test; Ctrl-C within 5 s to stop"; sleep 5; }
    LIM=(-n -Pn --max-retries 1 --host-timeout 60s --max-rate 20)
    #      name | caps | families | tests are the same for every variant
    VARIANTS=("1|none (today)||${FAM_UNIT}" "2|CAP_NET_RAW||${FAM_UNIT}" "3|CAP_NET_RAW||${FAM_UNIT} AF_NETLINK"
              "4|CAP_NET_RAW||${FAM_UNIT} AF_PACKET" "5|CAP_NET_RAW||${FAM_UNIT} AF_NETLINK AF_PACKET"
              "6|CAP_NET_RAW CAP_NET_ADMIN||${FAM_UNIT} AF_NETLINK AF_PACKET")
    for v in "${VARIANTS[@]}"; do
      IFS='|' read -r n capsname _ fam <<<"$v"
      caps="$capsname"; [[ "$n" == 1 ]] && caps=""
      variant "$n" "$caps" "$fam" "$GROUPS_UNIT"
      in_force_only | run_unit "$caps" "$fam" "$GROUPS_UNIT" -- python3 -I -
      tests=("iflist|--iflist" "udp default|${LIM[*]} -sU -p 53 $TARGET" "udp --privileged|--privileged ${LIM[*]} -sU -p 53 $TARGET"
             "udp --privileged --send-ip|--privileged --send-ip ${LIM[*]} -sU -p 53 $TARGET"
             "os detect default|${LIM[*]} -O -p 22,80 $TARGET" "os detect --privileged|--privileged ${LIM[*]} -O -p 22,80 $TARGET")
      for t in "${tests[@]}"; do
        say "variant $n test: ${t%%|*}"
        # shellcheck disable=SC2086
        result="$(run_unit "$caps" "$fam" "$GROUPS_UNIT" -- "$NM" ${t#*|} 2>&1 || true)"
        printf '%s\n' "$result" | head -n 45
        echo "signals: $(printf '%s\n' "$result" | grep -c -i -E 'requires root|operation not permitted|permission denied|dnet|raw socket|quitting|failed to' || true) refusal-like lines; exit-status line above if any"
      done
    done
  fi
fi

# --- 2. AF_PACKET ---------------------------------------------------------------------------------------------------------------------
if want 2; then
  say "2. PASSIVE LINK-LAYER CAPTURE on '$IFACE' (headers only, counts only, no promiscuous mode, nothing sent)"
  echo "While the main window runs, note what you did on the network (for example reconnected a phone): the template asks for it."
  variant P1 "" "$FAM_UNIT" "$GROUPS_UNIT"; probe capture_frames.py | run_unit "" "$FAM_UNIT" "$GROUPS_UNIT" -- python3 -I - "$IFACE" 2
  variant P2 "" "$FAM_UNIT AF_PACKET" "$GROUPS_UNIT"; probe capture_frames.py | run_unit "" "$FAM_UNIT AF_PACKET" "$GROUPS_UNIT" -- python3 -I - "$IFACE" 2
  variant P3 "CAP_NET_RAW" "$FAM_UNIT AF_PACKET" "$GROUPS_UNIT"; echo "window: ${WINDOW}s starts now"
  probe capture_frames.py | run_unit "CAP_NET_RAW" "$FAM_UNIT AF_PACKET" "$GROUPS_UNIT" -- python3 -I - "$IFACE" "$WINDOW"
fi

# --- 3. auth.log ----------------------------------------------------------------------------------------------------------------------
if want 3; then
  say "3. LOG FILE PERMISSIONS (no log line is read; only modes, owners and access checks)"
  stat -c '%n owner=%U:%G mode=%a' /var/log /var/log/auth.log /var/log/auth.log.1 /var/log/btmp /var/log/syslog 2>&1 | sed 's/^stat: //'
  echo "adm group: gid=$(getent group adm | cut -d: -f3) members=$(getent group adm | cut -d: -f4 | tr ',' '\n' | grep -c . || true)"
  echo "files the adm group can read under /var/log (depth 2):"
  find /var/log -maxdepth 2 -type f -group adm -perm -g=r -printf '  %p mode=%m owner=%u\n' 2>/dev/null | head -40
  echo "count readable through adm: $(find /var/log -maxdepth 2 -type f -group adm -perm -g=r 2>/dev/null | wc -l)"
  echo "logrotate create modes (the rotation that undid an ACL in ADR-0030):"
  grep -n -E '^[[:space:]]*(create|su) ' /etc/logrotate.d/rsyslog /etc/logrotate.d/btmp /etc/logrotate.conf 2>/dev/null || echo "  none found"
  CHECK='import os; [print("access(%s)=%s" % (p, os.access(p, os.R_OK))) for p in ("/var/log/auth.log","/var/log/syslog","/var/log/btmp")]'
  variant L1 "" "$FAM_UNIT" "$GROUPS_UNIT"; in_force_only | run_unit "" "$FAM_UNIT" "$GROUPS_UNIT" -- python3 -I -; run_unit "" "$FAM_UNIT" "$GROUPS_UNIT" -- python3 -I -c "$CHECK"
  variant L2 "" "$FAM_UNIT" "adm"; in_force_only | run_unit "" "$FAM_UNIT" "adm" -- python3 -I -; run_unit "" "$FAM_UNIT" "adm" -- python3 -I -c "$CHECK"
fi

# --- 4. netlink neighbour subscription ---------------------------------------------------------------------------------------------------
if want 4; then
  say "4. UNPRIVILEGED NETLINK NEIGHBOUR SUBSCRIPTION (a joined multicast group; nothing is sent)"
  variant N1 "" "$FAM_UNIT (the unit today)" "$GROUPS_UNIT"; probe neigh_listen.py | run_unit "" "$FAM_UNIT" "$GROUPS_UNIT" -- python3 -I - 2
  variant N2 "" "$FAM_UNIT AF_NETLINK" "$GROUPS_UNIT"; echo "passive window: ${WINDOW}s on the host (counts only: the normal rate of state changes, the noise baseline)"
  probe neigh_listen.py | run_unit "" "$FAM_UNIT AF_NETLINK" "$GROUPS_UNIT" -- python3 -I - "$WINDOW"
  if [[ $SIMULATE -eq 1 ]]; then
    say "4b. SIMULATED GATEWAY CHANGE in a throwaway network namespace (--simulate-neigh-change; the host's interfaces and routes are not touched)"
    if [[ $PRINT -eq 1 ]]; then echo "(would: ip netns add n0probe; dummy or veth; listener in the namespace; neigh add / replace / del of made-up 10.254.0.1; ip netns del n0probe)"
    else
      trap 'ip netns del n0probe 2>/dev/null || true' EXIT
      if ! ip netns add n0probe; then echo "(cannot create the namespace: simulation skipped)"
      else
        DEV=""
        if ip -n n0probe link add n0dummy type dummy 2>/dev/null; then DEV=n0dummy
        elif ip -n n0probe link add n0a type veth peer name n0b 2>/dev/null; then ip -n n0probe link set n0b up; DEV=n0a; fi
        if [[ -z "$DEV" ]]; then echo "(neither a dummy nor a veth interface could be created: simulation skipped)"
        else
          ip -n n0probe addr add 10.254.0.2/24 dev "$DEV"; ip -n n0probe link set "$DEV" up; ip -n n0probe link set lo up
          (probe neigh_listen.py | run_unit "" "$FAM_UNIT AF_NETLINK" "$GROUPS_UNIT" -p NetworkNamespacePath=/run/netns/n0probe -- python3 -I - 14 --show-addr) &
          LISTEN=$!; sleep 5
          echo "# add 10.254.0.1 -> 02:00:00:00:00:01 (the 'gateway'), then change its address, then delete it"
          ip -n n0probe neigh add 10.254.0.1 lladdr 02:00:00:00:00:01 dev "$DEV" nud reachable || echo "(neigh add failed)"; sleep 1
          ip -n n0probe neigh replace 10.254.0.1 lladdr 02:00:00:00:00:02 dev "$DEV" nud reachable || echo "(neigh replace failed)"; sleep 1
          ip -n n0probe neigh del 10.254.0.1 dev "$DEV" || echo "(neigh del failed)"
          wait "$LISTEN" || true
        fi
      fi
      ip netns del n0probe 2>/dev/null; trap - EXIT
    fi
  fi
fi

# --- 5. same-user finding, secondary check ------------------------------------------------------------------------------------------------
if want 5; then
  say "5. UNIX SOCKET AND LANDLOCK, SECONDARY CHECK in a transient unit (the primary check is the real agent terminal: see the audit, item 5)"
  variant U1 "" "$FAM_UNIT" "$GROUPS_UNIT"; probe unix_probe.py | run_unit "" "$FAM_UNIT" "$GROUPS_UNIT" -- python3 -I -
fi
say "DONE. Results file: ${OUT:-(print mode)}"
