#!/usr/bin/env bash
# Exercise the sourced UI only; never run host provisioning.
set -euo pipefail
cd "$(dirname "$0")/.."
export NO_COLOR=1
source ./setup.sh --yes
UI_BACKEND=ansi
INTERACTIVE=1
header="$(print_header)"
[[ "$header" == *"◖◗"* ]] || { echo 'FAIL: current split-disc brand mark missing'; exit 1; }
[[ "$header" == *"HOST SETUP"* ]] || { echo 'FAIL: setup identity missing'; exit 1; }
# A menu captured by its caller must return only the selected tag.
selected="$(ui_menu 'Actions' install 'Install' quit 'Quit' <<< '')"
[[ "$selected" == install ]] || { echo 'FAIL: menu output polluted'; exit 1; }
selected="$(ui_menu 'Actions' install 'Install' quit 'Quit' <<< $'\033[B\n')"
[[ "$selected" == quit ]] || { echo 'FAIL: keyboard navigation broken'; exit 1; }
[[ "$UI_BRAND" == '' ]] || { echo 'FAIL: piped output has colour'; exit 1; }
# Verify every native widget gets the brand identity, without launching newt.
whiptail() { [[ "$*" == *"--backtitle ◖◗ llm-proxy / HOST SETUP"* ]]; }
UI_BACKEND=whiptail
ui_menu Actions install Install
ui_input Path /tmp
ui_confirm Continue
ui_msgbox Done
ui_textbox /dev/null Logs
dialog() { [[ "$*" == *"--backtitle ◖◗ llm-proxy / HOST SETUP"* ]]; }
UI_BACKEND=dialog
ui_confirm Continue
# --- unattended mode ---------------------------------------------------------
# --unattended implies non-interactive and requires an explicit subcommand.
out="$(bash -c 'source ./setup.sh --install --unattended && printf "%s/%s/%s" "$UNATTENDED" "$INTERACTIVE" "$MODE"')"
[[ "$out" == "1/0/install" ]] || { echo "FAIL: --unattended parsing ($out)"; exit 1; }
if bash -c 'source ./setup.sh --unattended' >/dev/null 2>&1; then
  echo 'FAIL: --unattended without a subcommand should exit'; exit 1
fi
# --yes keeps its old contract: non-interactive, defaults, no unit reuse.
out="$(bash -c 'source ./setup.sh --install --yes && printf "%s/%s/%s" "$UNATTENDED" "$INTERACTIVE" "$MODE"')"
[[ "$out" == "0/0/install" ]] || { echo "FAIL: --yes parsing ($out)"; exit 1; }

# Previous settings come from the installed unit, not today's defaults.
unit="$(mktemp)"
printf 'User=svc\nEnvironment=LLM_PROXY_HOME=/srv/root\nExecStart=/opt/bin/llm-proxy\n' > "$unit"
SVC_USER=llm-proxy; SVC_ROOT=/var/lib/llm-proxy; INSTALL_BIN=/usr/local/bin/llm-proxy
resolve_unit_settings "$unit" || { echo 'FAIL: unit settings not resolved'; exit 1; }
[[ "$SVC_USER" == svc && "$SVC_ROOT" == /srv/root && "$INSTALL_BIN" == /opt/bin/llm-proxy ]] \
  || { echo 'FAIL: unit settings not reused'; exit 1; }
rm -f "$unit"
if resolve_unit_settings /nonexistent >/dev/null 2>&1; then
  echo 'FAIL: missing unit must not resolve'; exit 1
fi

# The unattended summary must name every value that will be applied.
SVC_USER=svc; SVC_ROOT=/tmp/llm-proxy-summary; INSTALL_BIN=/opt/bin/llm-proxy
DO_BUILD=1; START_NOW=0; STEP=0; UI_BACKEND=ansi
summary="$(print_settings 'previous install')"
[[ "$summary" == *"service user:    svc"* ]] || { echo 'FAIL: summary user'; exit 1; }
[[ "$summary" == *"data root:       /tmp/llm-proxy-summary"* ]] || { echo 'FAIL: summary root'; exit 1; }
[[ "$summary" == *"binary:          /opt/bin/llm-proxy"* ]] || { echo 'FAIL: summary binary'; exit 1; }
[[ "$summary" == *"build binary:    yes"* ]] || { echo 'FAIL: summary build'; exit 1; }
[[ "$summary" == *"start service:   no"* ]] || { echo 'FAIL: summary start'; exit 1; }

printf 'PASS: setup brand, output isolation, colour policy, native widgets, unattended reuse\n'
