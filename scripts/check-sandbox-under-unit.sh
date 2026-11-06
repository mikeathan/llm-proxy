#!/usr/bin/env bash
# Runs scripts/sandbox-probe.py as the service user under the SAME hardening as the installed llm-proxy unit, and prints what a jailed
# child can do there. A container or CI runner has no such unit, which is how the Unix-socket filter passed every test and still failed
# on its first real run (docs/architecture/pitfalls.md 68). Run it after any change to the sandbox, before deploying the binary.
#   sudo scripts/check-sandbox-under-unit.sh            run it
#   scripts/check-sandbox-under-unit.sh --print         show the systemd-run command only
set -euo pipefail

UNIT="${UNIT:-llm-proxy}"
HERE="$(cd "$(dirname "$0")" && pwd)"
TEMPLATE="$HERE/../docs/services/llm-proxy.service"

# The hardening directives that change what a child may do. They are read from the installed unit (so a unit that has drifted from the
# template is what gets tested), or from the repo's template when no unit is installed.
KEYS='NoNewPrivileges|ProtectSystem|ProtectHome|PrivateTmp|ProtectKernelTunables|ProtectKernelModules|ProtectKernelLogs|ProtectControlGroups|ProtectClock|ProtectHostname|RestrictNamespaces|RestrictRealtime|RestrictSUIDSGID|LockPersonality|MemoryDenyWriteExecute|SystemCallFilter|SystemCallArchitectures|SystemCallErrorNumber|RestrictAddressFamilies|CapabilityBoundingSet|AmbientCapabilities|SupplementaryGroups|User|Group|UMask'
if systemctl cat "$UNIT" >/dev/null 2>&1; then source_text="$(systemctl cat "$UNIT")"; from="the installed unit $UNIT"
else source_text="$(cat "$TEMPLATE")"; from="the repo template (no installed unit)"; fi

props=()
while IFS= read -r line; do props+=(-p "$line"); done < <(printf '%s\n' "$source_text" | grep -E "^(${KEYS})=" | sed 's/[[:space:]]*#.*$//')
[[ ${#props[@]} -gt 0 ]] || { echo "no hardening directives found in $from" >&2; exit 2; }

cmd=(systemd-run --pipe --wait --quiet "${props[@]}" -- python3 -)
if [[ "${1:-}" == "--print" ]]; then
  echo "# properties from $from"; printf '%q ' "${cmd[@]}"; echo " < $HERE/sandbox-probe.py"; exit 0
fi
echo "# properties from $from"
"${cmd[@]}" < "$HERE/sandbox-probe.py"
echo "# what the unit allows of the two routes:"
systemd-analyze syscall-filter @system-service 2>/dev/null | grep -wE 'seccomp|prctl' || echo "(neither seccomp nor prctl is listed in @system-service by name: prctl is reached through a group, seccomp is in @sandbox)"
