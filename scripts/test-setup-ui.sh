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
printf 'PASS: setup brand, output isolation, colour policy and native widgets\n'
