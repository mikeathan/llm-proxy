#!/usr/bin/env bash
# setup.sh — interactive installer/uninstaller for the llm-proxy systemd
# service (host provisioning). LINUX ONLY (systemd). macOS production
# deployment uses the dedicated-user launchd daemon instead —
# docs/services/llm-proxy.launchd.plist (+ docs/service_setup.md → macOS).
#
# UI: native `dialog`/`whiptail` panels when available; falls back to a
# plain-ANSI TUI otherwise. Zero required deps.
#
# Subcommands (TUI menu shown when run bare):
#   ./setup.sh install    install flow (build binary + register systemd)
#   ./setup.sh register   register systemd only (binary already built)
#   ./setup.sh build      build binary only
#   ./setup.sh uninstall  remove service (asks about data)
#   ./setup.sh purge      uninstall + erase EVERYTHING (typed confirmation)
#   ./setup.sh access     grant service-user read access to external model/
#                         binary paths (ACLs; explicit opt-in)
#   ./setup.sh service    start/stop/restart/status + view service logs
#
# Flags (for CI / unattended runs; skip the TUI entirely):
#   --install     install flow with defaults
#   --uninstall   uninstall flow with defaults (data is KEPT)
#   --build       build the binary first (scripts/build.sh)
#   --force       overwrite binary + unit even when identical
#   --yes         accept all prompts with defaults
#
# Contributor dev setup (gitleaks + git hooks) lives in
# scripts/setup-gitleaks.sh — different job, different machine.
set -euo pipefail

PRJ_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_NAME="llm-proxy"

# Defaults (overridable via interactive prompts)
SVC_USER="llm-proxy"
SVC_ROOT="/var/lib/llm-proxy"
INSTALL_BIN="/usr/local/bin/${BIN_NAME}"

FORCE=0; DO_BUILD=0; ASSUME_YES=0; PURGE=0; MODE=""

for arg in "$@"; do
  case "$arg" in
    install)     MODE="install" ;;
    register)    MODE="install" ;;
    uninstall)   MODE="uninstall" ;;
    purge)       MODE="purge" ;;
    build)       MODE="build" ;;
    service)     MODE="service" ;;
    access)      MODE="access" ;;
    --install)   MODE="install" ;;
    --uninstall) MODE="uninstall" ;;
    --build)     DO_BUILD=1 ;;
    --force)     FORCE=1 ;;
    --yes)       ASSUME_YES=1 ;;
    -h|--help)   sed -n '2,28p' "$0"; exit 0 ;;
    *) echo "unknown arg: $arg (-h for usage)" >&2; exit 1 ;;
  esac
done

INTERACTIVE=1
[[ $ASSUME_YES == 1 || -n "$MODE" || -t 0 ]] || {
  echo "non-interactive shell: pass --install/--uninstall (+--yes) or run from a terminal" >&2
  exit 1
}
[[ $ASSUME_YES == 1 ]] && INTERACTIVE=0

# =============================================================================
# UI LAYER
# Two backends implement the same five primitives. Logic below calls ONLY
# these; it never talks to dialog/whiptail/ANSI directly.
#   ui_menu    <title> <tag> <desc> ...   -> selected tag on stdout
#   ui_input   <prompt> <default>         -> entered value on stdout
#   ui_confirm <prompt>                   -> exit 0 = yes (default no if prompt
#                                            contains "[y/N]")
#   ui_msgbox  <text>                     -> blocking notice
#   ui_pause   <text>                     -> non-blocking one-line status
# =============================================================================

# Pick the richest available backend.
if command -v whiptail >/dev/null 2>&1; then
  UI_BACKEND="whiptail"
elif command -v dialog >/dev/null 2>&1; then
  UI_BACKEND="dialog"
else
  UI_BACKEND="ansi"
fi

# Shared ANSI palette + toolchain discovery. The palette lives in
# scripts/lib/ui.sh so setup.sh, build.sh and the other dev scripts cannot drift
# apart; ui.sh also honors NO_COLOR. Alias it to the short names this file uses.
# shellcheck source=scripts/lib/ui.sh
source "$PRJ_ROOT/scripts/lib/ui.sh"
# shellcheck source=scripts/lib/toolchain.sh
source "$PRJ_ROOT/scripts/lib/toolchain.sh"
BOLD=$UI_BOLD; ACCENT=$UI_MAUVE; TEXT=$UI_TEXT
GREEN=$UI_GREEN; YELLOW=$UI_YELLOW; RED=$UI_RED; DIM=$UI_DIM; NC=$UI_NC

# --- whiptail theme ---------------------------------------------------------
# Pin a Catppuccin Macchiato look (the theme this repo's opencode TUI uses):
# black panels, mauve borders/titles, light text, teal highlights. newt only
# takes the 16-colour names, so these are the nearest ANSI slots. Honor
# NO_COLOR to keep the system palette. (dialog uses dialogrc, not newt, so this
# only applies to the whiptail backend.)
if [[ $UI_BACKEND == whiptail && -z "${NO_COLOR:-}" ]]; then
  export NEWT_COLORS='
    root=white,black
    window=white,black
    border=magenta,black
    title=magenta,black
    textbox=white,black
    button=black,magenta
    compactbutton=black,magenta
    actbutton=black,cyan
    checkbox=white,black
    actcheckbox=black,cyan
    entry=white,black
    actentry=black,cyan
    label=white,black
    listbox=white,black
    actlistbox=black,cyan
    helpline=cyan,black
    roottext=cyan,black
  '
fi

info()    { printf '  %s•%s %s%s%s\n' "$ACCENT" "$NC" "$TEXT" "$1" "$NC"; }
success() { printf '  %s✓%s %s%s%s\n' "$GREEN" "$NC" "$TEXT" "$1" "$NC"; }
warn()    { printf '  %s!%s %s%s%s\n' "$YELLOW" "$NC" "$TEXT" "$1" "$NC"; }
fail()    { printf '  %s✗%s %s%s%s\n' "$RED" "$NC" "$TEXT" "$1" "$NC" >&2; exit 1; }

# --- whiptail / dialog backend ---------------------------------------------
# Sizing: menus/boxes get a fixed comfortable geometry; whiptail and dialog
# accept the same subset used here (title, height, width, list height).
# The 3>&1 1>&2 2>&3 dance swaps stdout/stderr so the rendered widget goes to
# the terminal while only the RESULT lands on stdout for $(...) capture.

ui_menu() { # $1 title, then tag/desc pairs
  local title="$1"; shift
  local items=("$@") list_h
  list_h=$(( (${#items[@]} / 2) > 12 ? 12 : ${#items[@]} / 2 ))
  case "$UI_BACKEND" in
    whiptail) whiptail --title "$title" --menu "" 20 70 "$list_h" "${items[@]}" 3>&1 1>&2 2>&3 ;;
    dialog)   dialog --clear --title "$title" --menu "" 20 70 "$list_h" "${items[@]}" 3>&1 1>&2 2>&3; clear ;;
    ansi)     ansi_menu "$title" "${items[@]}" ;;
  esac
}

# Arrow-key menu for the ANSI fallback. Renders to stderr, prints the selected
# TAG (odd positions) to stdout — same contract as whiptail/dialog backends.
ansi_menu() {
  local title="$1"; shift
  local items=("$@") n=$(( ${#items[@]} / 2 )) sel=1 i key seq
  if [[ $INTERACTIVE == 0 ]]; then echo "${items[0]}"; return; fi
  local esc_t=1; (( BASH_VERSINFO[0] > 3 )) && esc_t=0.1
  while true; do
    printf '%s%s%s%s\n' "$ACCENT" "$BOLD" "$title" "$NC" >&2
    for ((i=0; i<n; i++)); do
      local tag="${items[$((i*2))]}" desc="${items[$((i*2+1))]}"
      if (( i+1 == sel )); then
        printf '  %s%s▌ %s%s\n' "$ACCENT" "$BOLD" "$desc" "$NC" >&2
      else
        printf '%s    %s%s\n' "$TEXT" "$desc" "$NC" >&2
      fi
    done
    printf '%s  (↑/↓ + Enter, q to quit)%s\n' "$DIM" "$NC" >&2
    IFS= read -rsn1 key || exit 0
    if [[ $key == $'\x1b' ]]; then
      read -rsn2 -t "$esc_t" seq || true
      case "$seq" in
        '[A') (( sel = sel == 1 ? n : sel-1 )) ;;
        '[B') (( sel = sel == n ? 1 : sel+1 )) ;;
      esac
    elif [[ $key == "" ]]; then
      break
    elif [[ $key == "q" ]]; then
      echo -e "${DIM}bye.${NC}" >&2; exit 0
    fi
    printf '\033[%dA\033[J' $((n+2)) >&2
  done
  echo "${items[$(( (sel-1)*2 ))]}"
}

ui_input() { # $1 prompt, $2 default
  case "$UI_BACKEND" in
    whiptail) whiptail --title "llm-proxy" --inputbox "$1" 9 65 "$2" 3>&1 1>&2 2>&3 ;;
    dialog)   dialog --clear --title "llm-proxy" --inputbox "$1" 9 65 "$2" 3>&1 1>&2 2>&3; clear ;;
    ansi)     local val; read -r -e -p "$1 [$2] " val || true; echo "${val:-$2}" ;;
  esac
}

ui_confirm() {
  local default_no=0
  [[ "$1" == *"[y/N]"* ]] && default_no=1
  if [[ $INTERACTIVE == 0 ]]; then
    [[ $default_no == 0 ]]
  elif [[ $UI_BACKEND == ansi ]]; then
    local ans; read -r -p "  $1 " ans; [[ ! "$ans" =~ ^[Nn] ]]
  else
    case "$UI_BACKEND" in
      whiptail) whiptail --title "llm-proxy" --yesno "$1" 8 65 3>&1 1>&2 2>&3 ;;
      dialog)   dialog --clear --title "llm-proxy" --yesno "$1" 8 65 3>&1 1>&2 2>&3 ;;
    esac
  fi
}

ui_msgbox() { # $1 text
  if [[ $INTERACTIVE == 0 || $UI_BACKEND == ansi ]]; then
    echo "  $1"; return
  fi
  # Same fd swap as ui_menu/ui_input: newt/dialog draw on stdout, so without
  # the swap the widget renders into a command-substitution pipe (invisible)
  # and the script appears to hang. 20x70 so longer texts fit comfortably.
  case "$UI_BACKEND" in
    whiptail) whiptail --title "llm-proxy" --msgbox "$1" 20 70 3>&1 1>&2 2>&3 ;;
    dialog)   dialog --clear --title "llm-proxy" --msgbox "$1" 20 70 3>&1 1>&2 2>&3; clear ;;
  esac
}

# Display a file's contents in a scrollable textbox. Used for command output
# (systemctl status, journalctl) inside service_flow: printing to the
# terminal would be instantly covered by the next menu redraw.
ui_textbox() { # $1 = file, $2 = title
  local file="$1" title="${2:-llm-proxy}"
  case "$UI_BACKEND" in
    whiptail) whiptail --title "$title" --textbox "$file" 24 76 3>&1 1>&2 2>&3 ;;
    dialog)   dialog --clear --title "$title" --textbox "$file" 24 76 3>&1 1>&2 2>&3; clear ;;
    ansi)
      cat "$file"
      read -r -p "  (Enter to continue)" _ || true
      ;;
  esac
}

ui_pause() { # non-blocking status line (progress feedback during work)
  if [[ $UI_BACKEND == ansi ]]; then
    info "$1"
  else
    echo ">>> $1"
  fi
}

# =============================================================================
# LOGIC LAYER — calls ui_* only. All steps are idempotent.
# =============================================================================

check_env() {
  if ! command -v systemctl >/dev/null 2>&1; then
    if [[ "$(uname -s)" == "Darwin" ]]; then
      fail "macOS deployment is NOT covered by setup.sh (Linux/systemd only) — see docs/service_setup.md → macOS: dedicated-user launchd daemon via docs/services/llm-proxy.launchd.plist."
    fi
    fail "systemd not found — this script targets Linux servers (see docs/service_setup.md)."
  fi
  [[ -f "docs/services/${BIN_NAME}.service" ]] || fail "docs/services/${BIN_NAME}.service missing — run from the repo root."
  [[ $EUID -eq 0 ]] || fail "run with sudo: sudo ./setup.sh"
}

# -----------------------------------------------------------------------------
# SERVICE USER CHOICE — dedicated hardened account (default) vs the invoking
# user's own account. The screen spells out the trade-off before deciding.
# Returns the chosen username on stdout.
# -----------------------------------------------------------------------------
choose_service_user() {
  local invoking="${SUDO_USER:-$(id -un)}"
  if [[ $INTERACTIVE != 1 ]]; then
    echo "$SVC_USER" # non-interactive: keep the default (llm-proxy)
    return
  fi
  ui_msgbox "Who should run the llm-proxy service?

  DEDICATED USER (recommended — '${SVC_USER}')
    A locked system account (no password, no login) that owns nothing
    except the data root. If the proxy is ever compromised, the damage
    is contained: it cannot read your home directory unless you later
    grant access to specific model/binary paths (the installer offers
    that next, via read-only ACLs).

  YOUR OWN ACCOUNT ('${invoking}')
    No access grants ever needed — model dirs and binaries anywhere in
    your home just work. But the proxy and every agent session it spawns
    run with YOUR full identity: they can read everything you can
    (SSH keys, credentials, git tokens, your whole dev tree). A proxy
    bug or a runaway agent touches your files, not a sandbox.

  Pick YOUR OWN ACCOUNT only on a personal, single-user machine where
  the convenience outweighs the exposure."
  local choice
  choice="$(ui_menu "Run the service as:" \
    dedicated "${SVC_USER} — dedicated hardened account (recommended)" \
    current   "${invoking} — your own account (zero setup, broader exposure)" \
    custom    "Another username (system account will be created)")"
  case "$choice" in
    dedicated) echo "$SVC_USER" ;;
    current)   echo "$invoking" ;;
    custom)
      local u
      u="$(ui_input "System username:" "$SVC_USER")"
      [[ -n "$u" ]] || fail "service user cannot be empty"
      echo "$u"
      ;;
  esac
}

# -----------------------------------------------------------------------------
# EXTERNAL PATH ACCESS — explicit opt-in (./setup.sh access): let the service
# user read model dirs / binaries that live outside the data root.
#
# The unit keeps ProtectHome=read-only (no widening of the sandbox: read-only
# everywhere, writable only under the data root), so the only blocker is
# classic Unix permissions. We grant the service user ACLs:
#   - rX (read files, read+traverse dirs) recursively on the target path
#   - x (traverse only) on every parent up to / — reachability WITHOUT
#     listing rights, so /home/<user> stays private
# Nothing is copied or moved; the paths stay where the user put them.
grant_read_access() { # $1 = service user, $2 = path
  local user="$1" path="$2"
  if [[ ! -e "$path" ]]; then
    warn "not found: $path (skipping)"
    return
  fi
  if ! command -v setfacl >/dev/null 2>&1; then
    warn "setfacl not available — install the 'acl' package and re-run (skipping $path)"
    return
  fi
  setfacl -R -m "u:${user}:rX" "$path"
  # Traverse-only on the parent chain: stop at / (root is traversable already).
  local parent
  parent="$(dirname "$(realpath "$path")")"
  while [[ "$parent" != "/" ]]; do
    if setfacl -m "u:${user}:x" "$parent" 2>/dev/null; then
      info "  traverse granted on $parent"
    fi
    parent="$(dirname "$parent")"
  done
  success "${user} can now read $path (parents traverse-only)"
}

# -----------------------------------------------------------------------------
# ACCESS — grant the service user read access to external paths (models,
# llama-server binary). Explicit opt-in, never part of install.
# -----------------------------------------------------------------------------
access_flow() {
  offer_access_grants
  info "restart the service afterwards: sudo systemctl restart ${BIN_NAME}.service"
}

# Prompt for model dir + llama-server binary paths and grant the service user
# read access via ACLs. Used by install (dedicated/custom user chosen) and by
# the explicit `access` mode. Saves answers in $SVC_ROOT/.setup-paths (0600)
# so later runs prefill them. Never runs in non-interactive mode.
offer_access_grants() {
  local MODEL_DIR_PATH="" LLAMA_BIN_PATH=""
  local PATHS_FILE="$SVC_ROOT/.setup-paths"
  # Defaults from a previous run, so re-runs don't retype paths.
  if [[ -f "$PATHS_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$PATHS_FILE"
  fi
  if [[ $INTERACTIVE == 1 ]]; then
    step "External model & runtime paths"
    MODEL_DIR_PATH="$(ui_input "Model directory the service should read (blank to skip):" "${MODEL_DIR_PATH:-}")"
    LLAMA_BIN_PATH="$(ui_input "llama-server binary path (blank to skip):" "${LLAMA_BIN_PATH:-}")"
    [[ -z "$MODEL_DIR_PATH" && -z "$LLAMA_BIN_PATH" ]] && { info "nothing to do"; return; }
    [[ -n "$MODEL_DIR_PATH" ]] && grant_read_access "$SVC_USER" "$MODEL_DIR_PATH"
    [[ -n "$LLAMA_BIN_PATH" ]] && grant_read_access "$SVC_USER" "$LLAMA_BIN_PATH"
    printf 'MODEL_DIR_PATH=%q\nLLAMA_BIN_PATH=%q\n' "$MODEL_DIR_PATH" "$LLAMA_BIN_PATH" > "$PATHS_FILE"
    chmod 0600 "$PATHS_FILE"
    success "saved for next run: $PATHS_FILE"
  else
    info "non-interactive — grant manually with:"
    info "  setfacl -R -m u:${SVC_USER}:rX <path>   (+ traverse on parent dirs)"
  fi
}

# -----------------------------------------------------------------------------
# SERVICE CONTROL — start/stop/restart/status + logs, for visibility when the
# service has failed. Run with sudo (systemd/journalctl need root).
# -----------------------------------------------------------------------------
service_flow() {
  local UNIT_NAME="${BIN_NAME}.service"
  if ! command -v systemctl >/dev/null 2>&1; then
    fail "systemd not found on this host"
  fi
  # Every action's output is shown in a scrollable textbox, not printed to
  # the terminal: the menu redraws after each action and would cover it.
  local tmp out
  tmp="$(mktemp)"
  trap 'rm -f "$tmp"' RETURN
  while :; do
    local action
    action="$(ui_menu "Service control — ${UNIT_NAME}" \
      start   "Start the service" \
      stop    "Stop the service" \
      restart "Restart the service (clears failed-state first)" \
      status  "Status — active state, PID, enablement" \
      logs    "Logs — last 100 lines" \
      follow  "Logs — follow live (Ctrl+C to exit)" \
      back    "Back")"
    case "$action" in
      start | stop | restart)
        [[ $action == restart ]] && systemctl reset-failed "$UNIT_NAME" 2>/dev/null || true
        if out="$(systemctl "$action" "$UNIT_NAME" 2>&1)"; then
          printf 'service %s: OK\n%s' "$action" "${out:-}" > "$tmp"
        else
          printf 'service %s FAILED:\n\n%s\n\nCheck: journalctl -u %s -n 50 --no-pager' \
            "$action" "$out" "$UNIT_NAME" > "$tmp"
        fi
        ui_textbox "$tmp" "Service ${action}"
        ;;
      status)
        systemctl status "$UNIT_NAME" --no-pager >"$tmp" 2>&1 || true
        ui_textbox "$tmp" "Service status"
        ;;
      logs)
        journalctl -u "$UNIT_NAME" -n 100 --no-pager >"$tmp" 2>&1 || true
        ui_textbox "$tmp" "Last 100 log lines"
        ;;
      follow)
        info "following ${UNIT_NAME} — Ctrl+C to stop (service keeps running)"
        journalctl -u "$UNIT_NAME" -n 20 -f || true
        ;;
      *)
        return
        ;;
    esac
  done
}

# -----------------------------------------------------------------------------
# BUILD — scripts/build.sh is the single build implementation (setup.sh and
# launch.sh call it; it also runs standalone). Here we run it inside the active
# UI: dialog (and any backend with a streaming widget) shows the output live in
# a programbox; whiptail has no such widget, so its output is captured and
# shown in a textbox on failure. Without a terminal (redirected output, CI) the
# output streams directly.
# -----------------------------------------------------------------------------

BUILD_BOX_TITLE="Building ${BIN_NAME}"
BUILD_BOX_PROMPT="Compiling frontend assets + backend binary — this can take a few minutes..."
BUILD_LOG=""

# Self-heal repo ownership before a user-run build: earlier root-run builds
# leave root-owned files (frontend_dist etc.) that break the build with EACCES.
repair_root_owned() {
  [[ -n "${SUDO_USER:-}" ]] || return 0
  find "$PRJ_ROOT" -user root -print -quit 2>/dev/null | grep -q . || return 0
  ui_pause "repairing root-owned files in the repo (from older root-run builds)..."
  chown -R "$SUDO_USER":"$(id -gn "$SUDO_USER")" "$PRJ_ROOT"
  success "repo ownership repaired (was: root)"
}

# build_command [env-prefix] — the shell command that runs scripts/build.sh as
# the invoking user. Under sudo the toolchain lives in that user's login
# environment and sudo's reset PATH hides it, so go is located explicitly (some
# users only have go on PATH via an interactive shell config `bash -l` never
# reads). A captured build passes "NO_COLOR=1" so the log is plain text.
build_command() {
  local prefix="${1:-}${1:+ }" go_bin user_home
  local script="cd $(printf '%q' "$PRJ_ROOT") && ${prefix}BUILD_INLINE=1 ./scripts/build.sh"
  if [[ -z "${SUDO_USER:-}" ]]; then
    printf '%sBUILD_INLINE=1 bash %q' "$prefix" "$PRJ_ROOT/scripts/build.sh"
    return
  fi
  go_bin="$(sudo -u "$SUDO_USER" bash -lc 'command -v go' 2>/dev/null | tail -n1 || true)"
  # A login shell may print a banner before the path (and "" when go is not on
  # PATH at all), so only trust an executable result; otherwise scan the common
  # install locations.
  if [[ ! -x "$go_bin" ]]; then
    user_home="$(getent passwd "$SUDO_USER" 2>/dev/null | cut -d: -f6 || true)"
    go_bin="$(find_go_bin "$user_home" || true)"
  fi
  if [[ -n "$go_bin" ]]; then
    printf 'sudo -u %q env PATH=%q:%q bash -lc %q' "$SUDO_USER" "$(dirname "$go_bin")" "$PATH" "$script"
  else
    printf 'sudo -u %q bash -lc %q' "$SUDO_USER" "$script"
  fi
}

# build_stream_widget — the option this backend uses to stream a command's
# output, or nothing when it has none. whiptail (newt) has neither --programbox
# nor --progressbox on most builds; dialog has --programbox. The help text is
# captured first (not piped into grep) so an early grep exit cannot SIGPIPE the
# UI binary and, under pipefail, fake a "no widget" answer.
build_stream_widget() {
  local help opt
  help="$("$UI_BACKEND" --help 2>&1 || true)"
  for opt in --programbox --progressbox; do
    if grep -q -- "$opt" <<<"$help"; then
      printf '%s\n' "$opt"
      return 0
    fi
  done
  return 1
}

# build_stream_box — live output in the backend's streaming widget. Leaves the
# full output in BUILD_LOG and returns the build's status (nonzero when the box
# itself failed, even if the build succeeded).
build_stream_box() { # $1 = widget option
  BUILD_LOG="$(mktemp)"
  local cmd; cmd="$(build_command NO_COLOR=1)"
  if eval "$cmd" 2>&1 | tee "$BUILD_LOG" | \
      "$UI_BACKEND" --title "$BUILD_BOX_TITLE" "$1" "$BUILD_BOX_PROMPT" 24 76; then
    return 0
  fi
  # PIPESTATUS[0] is the build; a 0 there means the box, not the build, failed.
  local rc="${PIPESTATUS[0]}"
  [[ "$rc" == 0 ]] && rc=1
  return "$rc"
}

# build_capture_box — fallback when the backend has no streaming widget: run
# with the output captured and let build_step show it (textbox on failure).
build_capture_box() {
  BUILD_LOG="$(mktemp)"
  local cmd; cmd="$(build_command NO_COLOR=1)"
  eval "$cmd" >"$BUILD_LOG" 2>&1
}

# build_step — build through scripts/build.sh, keeping output inside the UI.
build_step() {
  repair_root_owned
  ui_pause "building (scripts/build.sh) — this can take a few minutes..."

  # Stream directly when there is no TUI to draw in: the ANSI fallback, a
  # non-interactive run, or output redirected to a file/pipe (CI).
  if [[ $UI_BACKEND == ansi || $INTERACTIVE == 0 || ! -t 1 ]]; then
    eval "$(build_command)" || fail "build failed (scripts/build.sh)"
    success "build complete"
    return 0
  fi

  local rc=0 widget
  widget="$(build_stream_widget || true)"
  if [[ -n "$widget" ]]; then
    build_stream_box "$widget" || rc=$?
  else
    build_capture_box || rc=$?
  fi
  if (( rc != 0 )); then
    ui_textbox "$BUILD_LOG" "Build failed"
    rm -f "$BUILD_LOG"
    fail "build failed"
  fi
  rm -f "$BUILD_LOG"
  success "build complete"
}

# -----------------------------------------------------------------------------
# INSTALL
# -----------------------------------------------------------------------------
install_flow() {
  step "Configuration"
  if [[ $INTERACTIVE == 1 ]]; then
    SVC_ROOT="$(ui_input "Data root:" "$SVC_ROOT")"
    INSTALL_BIN="$(ui_input "Binary install path:" "$INSTALL_BIN")"
    # Refuse values that would corrupt the unit or the filesystem.
    [[ "$SVC_ROOT" == "/" || -z "$SVC_ROOT" ]] && fail "data root must be a real directory (not / or empty)"
    [[ -n "$INSTALL_BIN" ]] || fail "binary install path cannot be empty"
    SVC_USER="$(choose_service_user)"
  fi
  info "user=$SVC_USER root=$SVC_ROOT bin=$INSTALL_BIN backend=$UI_BACKEND"
  START_NOW=1
  if [[ $INTERACTIVE == 1 ]]; then
    ui_confirm "Start the service when done?" || START_NOW=0
  fi

  step "Binary"
  if [[ $DO_BUILD == 1 ]]; then
    build_step
  fi
  SRC_BIN="$PRJ_ROOT/backend/${BIN_NAME}"
  [[ -x "$SRC_BIN" ]] || fail "$SRC_BIN not found — build it first (scripts/build.sh) or pick 'Install (with build)'."
  if [[ -f "$INSTALL_BIN" ]] && [[ $FORCE != 1 ]] && cmp -s "$SRC_BIN" "$INSTALL_BIN" 2>/dev/null; then
    success "already installed and identical: $INSTALL_BIN (skipping)"
  else
    install -m 0755 "$SRC_BIN" "$INSTALL_BIN"
    success "installed $INSTALL_BIN"
  fi

  step "Service user (${SVC_USER})"
  if id "$SVC_USER" &>/dev/null; then
    success "user exists (skipping)"
  else
    useradd --system --home-dir "$SVC_ROOT" --shell /usr/sbin/nologin "$SVC_USER"
    success "created system user ${SVC_USER} (no password, nologin — login-locked by design)"
  fi

  # GPU device access. The service launches llama-server as this user, so it
  # must be in the device groups: AMD ROCm /dev/kfd and /dev/dri/renderD* are
  # root:render, generic DRI /dev/dri/card* is root:video. Without them
  # llama.cpp enumerates no GPU and silently falls back to CPU, which then
  # collides with the unit's memory ceiling. Add whichever groups exist;
  # `usermod -aG` is idempotent for an existing member.
  local gpu_group gpu_groups=0
  if command -v getent >/dev/null 2>&1; then
    for gpu_group in render video; do
      if getent group "$gpu_group" >/dev/null 2>&1; then
        usermod -aG "$gpu_group" "$SVC_USER"
        success "added ${SVC_USER} to ${gpu_group} (GPU device access)"
        gpu_groups=1
      fi
    done
  fi
  [[ $gpu_groups == 1 ]] || warn "no render/video groups found — a GPU local model may not be visible to the service"

  # External paths (models, llama-server binary) only need explicit grants
  # when the service does NOT run as the invoking user — its own files are
  # readable by definition.
  if [[ "$SVC_USER" != "${SUDO_USER:-$(id -un)}" ]]; then
    offer_access_grants
  else
    step "External model & runtime paths"
    success "service runs as you — model dirs and binaries are readable, no grants needed"
  fi

  step "Data root (${SVC_ROOT})"
  if [[ -d "$SVC_ROOT" ]]; then
    # Fix ownership if the dir predates this install or was created by root —
    # the service runs as SVC_USER and must be able to write here.
    local owner="$(stat -c '%U' "$SVC_ROOT" 2>/dev/null || stat -f '%Su' "$SVC_ROOT" 2>/dev/null || echo "?")"
    if [[ "$owner" != "$SVC_USER" ]]; then
      chown -R "$SVC_USER":"$SVC_USER" "$SVC_ROOT"
      success "exists — ownership corrected to ${SVC_USER} (was: ${owner})"
    else
      success "exists, owned by ${SVC_USER} (skipping)"
    fi
  else
    install -d -m 0700 -o "$SVC_USER" -g "$SVC_USER" "$SVC_ROOT"
    success "created (0700, owned by ${SVC_USER})"
  fi

  step "Legacy state migration"
  LEGACY_HOME=""
  command -v getent >/dev/null 2>&1 && LEGACY_HOME="$(getent passwd "${SUDO_USER:-}" 2>/dev/null | cut -d: -f6 || true)"
  LEGACY_DIR="${LEGACY_HOME:-}/.config/${BIN_NAME}"
  if [[ -n "$LEGACY_HOME" && -d "$LEGACY_DIR" ]]; then
    if [[ -f "$SVC_ROOT/settings.yml" ]]; then
      success "legacy dir found but $SVC_ROOT is already initialized (skipping)"
    elif ui_confirm "Found legacy state at $LEGACY_DIR — copy it into $SVC_ROOT?"; then
      cp -a "$LEGACY_DIR/." "$SVC_ROOT/"
      chown -R "$SVC_USER":"$SVC_USER" "$SVC_ROOT"
      success "state migrated (original left untouched)"
    fi
  else
    success "no legacy state found (skipping)"
  fi

  step "settings.yml → workspaces_dir"
  SETTINGS="$SVC_ROOT/settings.yml"
  if [[ ! -f "$SETTINGS" ]]; then
    info "no settings.yml yet — writing a stub"
    printf 'workspaces_dir: workspaces\n' > "$SETTINGS"
    chown "$SVC_USER":"$SVC_USER" "$SETTINGS"; chmod 0600 "$SETTINGS"
    success "created settings.yml with workspaces_dir: workspaces"
  # A present-but-EMPTY value (workspaces_dir: "") is treated as unset by the
  # backend and falls through to the default, so require a non-empty value
  # before declaring the setting done.
  elif grep -Eq '^workspaces_dir:[[:space:]]*[^[:space:]#]+' "$SETTINGS" 2>/dev/null; then
    success "workspaces_dir already set (skipping)"
  elif ui_confirm "Append 'workspaces_dir: workspaces' to $SETTINGS?"; then
    # Guard against appending to a file without a trailing newline (would
    # otherwise merge into the last YAML line).
    if [[ -s "$SETTINGS" ]] && [[ -n "$(tail -c1 "$SETTINGS")" ]]; then
      echo >> "$SETTINGS"
    fi
    echo 'workspaces_dir: workspaces' >> "$SETTINGS"
    success "appended workspaces_dir: workspaces"
  else
    warn "left as-is — agent workspaces may hit a read-only \$HOME under ProtectHome"
  fi

  step "Unit file"
  UNIT="/etc/systemd/system/${BIN_NAME}.service"
  # Render the unit template with the configured user/root. The template
  # ships with defaults; the five tokens below are the only host-specific
  # values. Custom SVC_USER/SVC_ROOT prompts would otherwise produce a unit
  # that runs the wrong user / wrong data root.
  #
  # Hardening is NOT rendered here — it travels with the template so it cannot
  # drift. In particular the template's SystemCallFilter=@system-service also
  # lists the three Landlock syscalls (landlock_create_ruleset,
  # landlock_add_rule, landlock_restrict_self) that the agent OS-sandboxing
  # layer probes at boot and uses to jail agent children. systemd's default
  # action on a non-allowlisted syscall is to KILL the process, so a systemd
  # whose @system-service predates Landlock would crash-loop the service with
  # "code=dumped, status=31/SYS" (SIGSYS). Keep them in the template; editing
  # the template and re-running this flow reinstalls the unit (the cmp below
  # detects the change).
  local RENDERED="/tmp/${BIN_NAME}.rendered.service"
  sed -e "s|^User=.*|User=${SVC_USER}|" \
      -e "s|^Group=.*|Group=${SVC_USER}|" \
      -e "s|^Environment=LLM_PROXY_HOME=.*|Environment=LLM_PROXY_HOME=${SVC_ROOT}|" \
      -e "s|^WorkingDirectory=.*|WorkingDirectory=${SVC_ROOT}|" \
      -e "s|^ReadWritePaths=.*|ReadWritePaths=${SVC_ROOT}|" \
      "docs/services/${BIN_NAME}.service" > "$RENDERED"
  if [[ -f "$UNIT" ]] && [[ $FORCE != 1 ]] && cmp -s "$RENDERED" "$UNIT" 2>/dev/null; then
    rm -f "$RENDERED"
    success "already installed and identical (skipping)"
  else
    install -m 0644 "$RENDERED" "$UNIT" && rm -f "$RENDERED"
    systemctl daemon-reload
    success "installed (User=$SVC_USER, root=$SVC_ROOT) + daemon-reloaded"
  fi

  if [[ $START_NOW == 1 ]]; then
    step "Enable & start"
    # Clear failed-state / start-limit residue (e.g. a long crash-loop under
    # the previous unit) so restart is not refused with "start request
    # repeated too quickly".
    systemctl reset-failed "$BIN_NAME.service" 2>/dev/null || true
    systemctl enable "$BIN_NAME.service" &>/dev/null
    if ! systemctl restart "$BIN_NAME.service"; then
      warn "restart failed — recent logs:"
      journalctl -u "$BIN_NAME.service" -n 10 --no-pager | sed 's/^/    /' >&2
      fail "service failed to start — see: journalctl -u ${BIN_NAME}.service -f"
    fi
    success "service enabled and (re)started"

    step "Verify"
    sleep 2
    if systemctl is-active --quiet "$BIN_NAME.service"; then
      success "service is active — PID $(systemctl show -p MainPID --value "$BIN_NAME.service")"
    else
      warn "service is NOT active — recent logs:"
      journalctl -u "$BIN_NAME.service" -n 10 --no-pager | sed 's/^/    /' >&2
      fail "check: journalctl -u ${BIN_NAME}.service -f"
    fi
  else
    info "start skipped — enable later with: sudo systemctl enable --now ${BIN_NAME}"
  fi

  if command -v systemctl >/dev/null 2>&1 && command -v systemd-analyze >/dev/null 2>&1; then
    SCORE="$(systemd-analyze security "$BIN_NAME.service" 2>/dev/null | tail -1 || true)"
    info "hardening: ${SCORE:-n/a}"
  fi

  # Refresh the dev launcher's env file so ./launch.sh defaults to the same
  # data root the service uses. Values are defaults only — anything exported
  # before ./launch.sh wins.
  printf '# generated by setup.sh — refreshed on every install\nLLM_PROXY_HOME=%q\n' "$SVC_ROOT" > "$PRJ_ROOT/.launch.env"
  success "dev launcher env refreshed: .launch.env (LLM_PROXY_HOME=$SVC_ROOT)"

  ui_msgbox "Install complete.
UI:    http://<host>:4001
Logs:  journalctl -u ${BIN_NAME}.service -f
Data:  ${SVC_ROOT} (single root: settings, DB, logs, workspaces)"
}

# -----------------------------------------------------------------------------
# UNINSTALL — also serves the purge flow (PURGE=1: no per-item asks, erase all)
# -----------------------------------------------------------------------------
uninstall_flow() {
  local UNIT="/etc/systemd/system/${BIN_NAME}.service"
  # Read the actually-installed user/root/binary so we tear down what was
  # installed, not today's defaults (a custom binary path survives this way).
  if [[ -f "$UNIT" ]]; then
    local u r b
    u="$(sed -n 's/^User=//p' "$UNIT" 2>/dev/null || true)"
    r="$(sed -n 's/^Environment=LLM_PROXY_HOME=//p' "$UNIT" 2>/dev/null || true)"
    b="$(sed -n 's/^ExecStart=//p' "$UNIT" 2>/dev/null || true)"
    [[ -n "$u" ]] && SVC_USER="$u"
    [[ -n "$r" ]] && SVC_ROOT="$r"
    [[ -n "$b" ]] && INSTALL_BIN="$b"
  fi

  step "Artifacts found"
  [[ -f "$UNIT" ]]           && info "unit:         $UNIT"        || info "unit:         (not installed)"
  [[ -f "$INSTALL_BIN" ]]    && info "binary:       $INSTALL_BIN"  || info "binary:       (not installed)"
  [[ -d "$SVC_ROOT" ]]       && info "data root:    $SVC_ROOT"     || info "data root:    (not present)"
  id "$SVC_USER" &>/dev/null && info "service user: $SVC_USER"     || info "service user: (not present)"

  step "Stop & disable service"
  if [[ -f "$UNIT" ]]; then
    systemctl stop "$BIN_NAME.service" 2>/dev/null || true
    systemctl disable "$BIN_NAME.service" 2>/dev/null || true
    systemctl reset-failed "$BIN_NAME.service" 2>/dev/null || true
    success "service stopped and disabled"
  else
    success "nothing to stop (skipping)"
  fi

  step "Remove unit file"
  if [[ -f "$UNIT" ]]; then
    rm -f "$UNIT"
    systemctl daemon-reload
    success "unit removed + daemon-reloaded"
  else
    success "no unit file (skipping)"
  fi

  step "Binary"
  if [[ -f "$INSTALL_BIN" ]] && { [[ $PURGE == 1 ]] || ui_confirm "Remove $INSTALL_BIN? [y/N]"; }; then
    rm -f "$INSTALL_BIN"
    success "binary removed"
  else
    info "binary kept"
  fi

  step "Data root (${SVC_ROOT})"
  if [[ ! -d "$SVC_ROOT" ]]; then
    success "not present (skipping)"
  elif [[ $PURGE != 1 ]] && ui_confirm "Keep data at $SVC_ROOT (recommended — DB, secrets, master.key)?"; then
    info "data kept — reinstalling later picks it up automatically"
  elif [[ $PURGE == 1 ]] || ui_confirm "Really delete everything under $SVC_ROOT? IRREVERSIBLE. [y/N]"; then
      # PURGE mode was already gated by the typed PURGE in purge_flow —
      # do not ask twice. Plain uninstall types DELETE here.
      if [[ $PURGE == 1 ]]; then
        rm -rf "$SVC_ROOT"
        success "data root erased (purge)"
      else
        local d=""
        if [[ $UI_BACKEND == ansi ]]; then
          read -r -p "  type DELETE to confirm: " d || true
        else
          d="$(ui_input "Type DELETE to confirm:" "")"
        fi
        if [[ "$d" == "DELETE" ]]; then
          rm -rf "$SVC_ROOT"
          success "data root deleted"
        else
          warn "confirmation did not match DELETE — data kept"
        fi
      fi
  else
    warn "deletion aborted — data kept"
  fi

  step "Service user (${SVC_USER})"
  if id "$SVC_USER" &>/dev/null; then
    if [[ -d "$SVC_ROOT" ]]; then
      info "kept — user stays (it owns $SVC_ROOT)"
    elif [[ "$SVC_USER" == "${SUDO_USER:-}" ]]; then
      # Never delete the invoking user's own account — the service may have
      # been installed to run as them (self-profile choice).
      warn "kept — $SVC_USER is the invoking user (refusing to delete)"
    elif [[ $PURGE == 1 ]] || ui_confirm "Remove system user $SVC_USER? [y/N]"; then
      userdel "$SVC_USER"
      success "user removed"
    fi
  else
    success "not present (skipping)"
  fi

  # Purge-only: also erase the legacy pre-relocation config tree.
  if [[ $PURGE == 1 ]]; then
    step "Legacy config (~/.config/${BIN_NAME})"
    LEGACY_HOME=""
    command -v getent >/dev/null 2>&1 && LEGACY_HOME="$(getent passwd "${SUDO_USER:-}" 2>/dev/null | cut -d: -f6 || true)"
    local legacy_dir="${LEGACY_HOME:-}/.config/${BIN_NAME}"
    if [[ -n "$LEGACY_HOME" && -d "$legacy_dir" ]]; then
      rm -rf "$legacy_dir"
      success "legacy config erased: $legacy_dir"
    else
      success "not present (skipping)"
    fi
  fi

  if [[ $PURGE == 1 ]]; then
    ui_msgbox "Purge complete. No llm-proxy artifacts remain."
  else
    ui_msgbox "Uninstall complete."
  fi
}

# -----------------------------------------------------------------------------
# PURGE — uninstall + erase everything, with a full-screen warning up front
# -----------------------------------------------------------------------------
purge_flow() {
  local UNIT="/etc/systemd/system/${BIN_NAME}.service"
  # Resolve what's actually installed for the warning listing.
  if [[ -f "$UNIT" ]]; then
    local u r b
    u="$(sed -n 's/^User=//p' "$UNIT" 2>/dev/null || true)"
    r="$(sed -n 's/^Environment=LLM_PROXY_HOME=//p' "$UNIT" 2>/dev/null || true)"
    b="$(sed -n 's/^ExecStart=//p' "$UNIT" 2>/dev/null || true)"
    [[ -n "$u" ]] && SVC_USER="$u"
    [[ -n "$r" ]] && SVC_ROOT="$r"
    [[ -n "$b" ]] && INSTALL_BIN="$b"
  fi

  local warning="FULL PURGE — this erases ALL llm-proxy artifacts:

  • service:        stop, disable, remove unit file
  • binary:         ${INSTALL_BIN}
  • DATA ROOT:      ${SVC_ROOT}
    (database, settings, provider secrets, master.key, workspaces)
  • service user:   ${SVC_USER}
  • legacy config:  ~/.config/${BIN_NAME} (if present)

This is IRREVERSIBLE. Workspaces are agent files — if any work
product lives there, it is gone forever.\n\nContinue only if you are certain."

  # Hard gate 1: explicit warning acceptance.
  if ! ui_confirm "$warning Continue? [y/N]"; then
    warn "purge aborted — nothing was erased"
    return
  fi
  # Hard gate 2: typed PURGE (works in both backends).
  local d=""
  if [[ $UI_BACKEND == ansi ]]; then
    read -r -p "  type PURGE to erase everything: " d
  else
    d="$(ui_input "Type PURGE to erase everything:" "")"
  fi
  if [[ "$d" != "PURGE" ]]; then
    warn "confirmation did not match PURGE — nothing was erased"
    return
  fi

  warn "full purge starting — everything listed above will be erased"
  PURGE=1 uninstall_flow
}

# =============================================================================
# FLOW CONTROL
# =============================================================================
STEP=0
step() {
  STEP=$((STEP+1))
  if [[ $UI_BACKEND == ansi ]]; then
    printf '\n%s▌%s %s[%d]%s %s%s%s\n' "$ACCENT" "$NC" "$DIM" "$STEP" "$NC" "$BOLD$TEXT" "$1" "$NC"
  else
    echo -e "\n>>> [ $STEP ] $1"
  fi
}

print_header() {
  if [[ $UI_BACKEND == ansi ]]; then
    printf '\n%s╭─%s %s%sllm-proxy%s %s· host setup%s\n' \
      "$ACCENT" "$NC" "$BOLD" "$TEXT" "$NC" "$DIM" "$NC"
    printf '%s│%s  %ssystemd installer · Linux only%s\n' "$ACCENT" "$NC" "$DIM" "$NC"
    printf '%s╰%s' "$ACCENT" "$NC"; ui_rule 54
  else
    echo "llm-proxy — host setup (UI: $UI_BACKEND)"
  fi
}

main() {
  print_header
  check_env

  case "$MODE" in
    install)   install_flow ;;
    uninstall) uninstall_flow ;;
    purge)     purge_flow ;;
    build)     build_step ;;
    service)   service_flow ;;
    access)    access_flow ;;
    *)
      case "$(ui_menu "What would you like to do?" \
        install           "Install — build binary + register systemd service" \
        register          "Register systemd only (binary already built)" \
        build             "Build binary only" \
        service           "Service — start / stop / restart / status / logs" \
        access            "Access — grant service read access to model/binary paths" \
        uninstall         "Uninstall — remove service (asks about data)" \
        purge             "Full purge — uninstall + erase EVERYTHING" \
        quit              "Quit")" in
        install)           DO_BUILD=1; install_flow ;;
        register)          install_flow ;;
        build)             build_step ;;
        service)           service_flow ;;
        access)            access_flow ;;
        uninstall)         if ui_confirm "Uninstall the llm-proxy service? [y/N]"; then uninstall_flow; else warn "aborted"; fi ;;
        purge)             purge_flow ;;
        *) exit 0 ;;
      esac
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
