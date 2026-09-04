#!/usr/bin/env bash
# setup.sh — interactive installer/uninstaller for the llm-proxy systemd
# service (host provisioning).
#
# UI: native `dialog`/`whiptail` panels (classic blue setup-screen look) when
# available; falls back to a plain-ANSI TUI otherwise. Zero required deps.
#
# Subcommands (TUI menu shown when run bare):
#   ./setup.sh install    install flow (build binary + register systemd)
#   ./setup.sh register   register systemd only (binary already built)
#   ./setup.sh build      build binary only
#   ./setup.sh uninstall  remove service (asks about data)
#   ./setup.sh purge      uninstall + erase EVERYTHING (typed confirmation)
#   ./setup.sh preview    dry-run preview of install
#
# Flags (for CI / unattended runs; skip the TUI entirely):
#   --install     install flow with defaults
#   --uninstall   uninstall flow with defaults (data is KEPT)
#   --build       build the binary first (scripts/build.sh)
#   --force       overwrite binary + unit even when identical
#   --dry-run     preview — no changes, works on macOS too
#   --yes         accept all prompts with defaults
#
# Contributor dev setup (gitleaks + git hooks) lives in
# scripts/setup-gitleaks.sh — different job, different machine.
set -euo pipefail

PRJ_ROOT="$(cd "$(dirname "$0")" && pwd)"
BIN_NAME="llm-proxy"

# Defaults (overridable via interactive prompts)
SVC_USER="llm-proxy"
SVC_ROOT="/var/lib/llm-proxy"
INSTALL_BIN="/usr/local/bin/${BIN_NAME}"

FORCE=0; DRY_RUN=0; DO_BUILD=0; ASSUME_YES=0; PURGE=0; MODE=""

for arg in "$@"; do
  case "$arg" in
    install)     MODE="install" ;;
    register)    MODE="install" ;;
    uninstall)   MODE="uninstall" ;;
    purge)       MODE="purge" ;;
    build)       MODE="build" ;;
    preview)     MODE="preview"; DRY_RUN=1 ;;
    --install)   MODE="install" ;;
    --uninstall) MODE="uninstall" ;;
    --build)     DO_BUILD=1 ;;
    --force)     FORCE=1 ;;
    --dry-run)   DRY_RUN=1 ;;
    --yes)       ASSUME_YES=1 ;;
    -h|--help)   sed -n '2,27p' "$0"; exit 0 ;;
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

# Colors (ANSI backend only; matches scripts/build.sh)
BOLD='\033[1m'; CYAN='\033[0;36m'; GREEN='\033[0;32m'
YELLOW='\033[0;33m'; RED='\033[0;31m'; DIM='\033[2m'; NC='\033[0m'

info()    { echo "  • $1"; }
success() { echo "  ✓ $1"; }
warn()    { echo "  ! $1"; }
fail()    { echo "  ✗ $1" >&2; exit 1; }

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
    echo -e "${CYAN}${BOLD}$title${NC}" >&2
    for ((i=0; i<n; i++)); do
      local tag="${items[$((i*2))]}" desc="${items[$((i*2+1))]}"
      if (( i+1 == sel )); then
        echo -e "  ${CYAN}${BOLD}▸ ${desc}${NC}" >&2
      else
        echo -e "    ${desc}" >&2
      fi
    done
    echo -e "${DIM}  (↑/↓ + Enter, q to quit)${NC}" >&2
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
  if [[ $DRY_RUN == 1 ]]; then return 1; fi
  if [[ $INTERACTIVE == 0 ]]; then
    [[ $default_no == 0 ]]
  elif [[ $UI_BACKEND == ansi ]]; then
    local ans; read -r -p "  $1 " ans; [[ ! "$ans" =~ ^[Nn] ]]
  else
    case "$UI_BACKEND" in
      whiptail) whiptail --title "llm-proxy" --yesno "$1" 8 65 ;;
      dialog)   dialog --clear --title "llm-proxy" --yesno "$1" 8 65 ;;
    esac
  fi
}

ui_msgbox() { # $1 text
  if [[ $DRY_RUN == 1 || $INTERACTIVE == 0 || $UI_BACKEND == ansi ]]; then
    echo "  $1"; return
  fi
  case "$UI_BACKEND" in
    whiptail) whiptail --title "llm-proxy" --msgbox "$1" 11 65 ;;
    dialog)   dialog --clear --title "llm-proxy" --msgbox "$1" 11 65 ;;
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

run() { if [[ $DRY_RUN == 1 ]]; then info "(dry-run) $*"; else "$@"; fi; }

# Confirm with dry-run/interactivity semantics applied; destructive branches
# are skipped during preview.
confirm() {
  if [[ $DRY_RUN == 1 ]]; then info "(dry-run) would prompt: $1"; return 1; fi
  ui_confirm "$1"
}

check_env() {
  if ! command -v systemctl >/dev/null 2>&1; then
    if [[ $DRY_RUN == 1 ]]; then
      warn "systemd not found on this host — previewing Linux install steps only"
    elif [[ -t 0 && $INTERACTIVE == 1 ]]; then
      # Preview-capable host (e.g. a dev Mac): offer the TUI in dry-run
      # instead of refusing outright.
      warn "systemd not found on this host — switching to preview mode (dry-run)"
      DRY_RUN=1
    else
      fail "systemd not found — this script targets Linux servers (see docs/service_setup)."
    fi
  fi
  [[ -f "docs/services/${BIN_NAME}.service" ]] || fail "docs/services/${BIN_NAME}.service missing — run from the repo root."
  if [[ $DRY_RUN == 1 ]]; then
    warn "DRY RUN — no changes will be made"
    [[ $EUID -eq 0 ]] || warn "not root: root-only paths cannot be inspected; output is approximate"
  elif [[ $EUID -ne 0 ]]; then
    fail "run with sudo: sudo ./setup.sh"
  fi
}

# -----------------------------------------------------------------------------
# INSTALL
# -----------------------------------------------------------------------------
install_flow() {
  step "Configuration"
  step "Configuration"
  if [[ $INTERACTIVE == 1 && $DRY_RUN != 1 ]]; then
    SVC_USER="$(ui_input "Service user:" "$SVC_USER")"
    SVC_ROOT="$(ui_input "Data root:" "$SVC_ROOT")"
    INSTALL_BIN="$(ui_input "Binary install path:" "$INSTALL_BIN")"
    # Refuse values that would corrupt the unit or the filesystem.
    [[ -n "$SVC_USER" ]] || fail "service user cannot be empty"
    [[ "$SVC_ROOT" == "/" || -z "$SVC_ROOT" ]] && fail "data root must be a real directory (not / or empty)"
    [[ -n "$INSTALL_BIN" ]] || fail "binary install path cannot be empty"
  fi
  info "user=$SVC_USER root=$SVC_ROOT bin=$INSTALL_BIN backend=$UI_BACKEND"
  START_NOW=1
  if [[ $INTERACTIVE == 1 && $DRY_RUN != 1 ]]; then
    confirm "Start the service when done?" || START_NOW=0
  fi

  step "Binary"
  if [[ $DO_BUILD == 1 ]]; then
    ui_pause "building (scripts/build.sh) — this can take a few minutes..."
    if [[ $DRY_RUN == 1 ]]; then
      info "(dry-run) bash $PRJ_ROOT/scripts/build.sh"
    elif [[ -n "${SUDO_USER:-}" ]]; then
      # Self-heal repo ownership: earlier versions of this script built as
      # root, leaving root-owned files (frontend_dist etc.) that break user-
      # run builds with EACCES. Repair before dropping to the invoking user.
      local root_owned
      root_owned="$(find "$PRJ_ROOT" -user root -print -quit 2>/dev/null || true)"
      if [[ -n "$root_owned" ]]; then
        ui_pause "repairing root-owned files in the repo (from older root-run builds)..."
        chown -R "$SUDO_USER":"$SUDO_USER" "$PRJ_ROOT"
        success "repo ownership repaired (was: root)"
      fi
      # Run the build as the invoking user: the toolchain (go, node) lives in
      # that user's login environment, and sudo's reset PATH hides it from
      # root. Login shell (-l) so profile-provided tool paths resolve.
      sudo -u "$SUDO_USER" bash -lc "cd '$PRJ_ROOT' && ./scripts/build.sh"
    else
      bash "$PRJ_ROOT/scripts/build.sh"
    fi
  fi
  SRC_BIN="$PRJ_ROOT/backend/${BIN_NAME}"
  [[ -x "$SRC_BIN" ]] || fail "$SRC_BIN not found — build it first (scripts/build.sh) or pick 'Install (with build)'."
  if [[ -f "$INSTALL_BIN" ]] && [[ $FORCE != 1 ]] && cmp -s "$SRC_BIN" "$INSTALL_BIN" 2>/dev/null; then
    success "already installed and identical: $INSTALL_BIN (skipping)"
  else
    run install -m 0755 "$SRC_BIN" "$INSTALL_BIN"
    success "installed $INSTALL_BIN"
  fi

  step "Service user (${SVC_USER})"
  if id "$SVC_USER" &>/dev/null; then
    success "user exists (skipping)"
  else
    run useradd --system --home-dir "$SVC_ROOT" --shell /usr/sbin/nologin "$SVC_USER"
    success "created system user ${SVC_USER} (no password, nologin — login-locked by design)"
  fi

  step "Data root (${SVC_ROOT})"
  if [[ -d "$SVC_ROOT" ]]; then
    # Fix ownership if the dir predates this install or was created by root —
    # the service runs as SVC_USER and must be able to write here.
    local owner="$(stat -c '%U' "$SVC_ROOT" 2>/dev/null || stat -f '%Su' "$SVC_ROOT" 2>/dev/null || echo "?")"
    if [[ "$owner" != "$SVC_USER" ]]; then
      run chown -R "$SVC_USER":"$SVC_USER" "$SVC_ROOT"
      success "exists — ownership corrected to ${SVC_USER} (was: ${owner})"
    else
      success "exists, owned by ${SVC_USER} (skipping)"
    fi
  else
    run install -d -m 0700 -o "$SVC_USER" -g "$SVC_USER" "$SVC_ROOT"
    success "created (0700, owned by ${SVC_USER})"
  fi

  step "Legacy state migration"
  LEGACY_HOME=""
  command -v getent >/dev/null 2>&1 && LEGACY_HOME="$(getent passwd "${SUDO_USER:-}" 2>/dev/null | cut -d: -f6 || true)"
  LEGACY_DIR="${LEGACY_HOME:-}/.config/${BIN_NAME}"
  if [[ -n "$LEGACY_HOME" && -d "$LEGACY_DIR" ]]; then
    if [[ -f "$SVC_ROOT/settings.yml" ]]; then
      success "legacy dir found but $SVC_ROOT is already initialized (skipping)"
    elif confirm "Found legacy state at $LEGACY_DIR — copy it into $SVC_ROOT?"; then
      run cp -a "$LEGACY_DIR/." "$SVC_ROOT/"
      run chown -R "$SVC_USER":"$SVC_USER" "$SVC_ROOT"
      success "state migrated (original left untouched)"
    fi
  else
    success "no legacy state found (skipping)"
  fi

  step "settings.yml → workspaces_dir"
  SETTINGS="$SVC_ROOT/settings.yml"
  if [[ ! -f "$SETTINGS" ]]; then
    if [[ $DRY_RUN == 1 ]]; then
      info "(dry-run) no settings.yml — would create a stub with workspaces_dir: workspaces"
    else
      info "no settings.yml yet — writing a stub"
      printf 'workspaces_dir: workspaces\n' > "$SETTINGS"
      chown "$SVC_USER":"$SVC_USER" "$SETTINGS"; chmod 0600 "$SETTINGS"
      success "created settings.yml with workspaces_dir: workspaces"
    fi
  elif grep -q '^workspaces_dir:' "$SETTINGS" 2>/dev/null; then
    success "workspaces_dir already set (skipping)"
  elif confirm "Append 'workspaces_dir: workspaces' to $SETTINGS?"; then
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
  local RENDERED="/tmp/${BIN_NAME}.rendered.service"
  if [[ $DRY_RUN == 1 ]]; then
    info "(dry-run) render unit (User=$SVC_USER, LLM_PROXY_HOME=$SVC_ROOT) and install to $UNIT"
  else
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
  fi

  if [[ $START_NOW == 1 ]]; then
    step "Enable & start"
    if [[ $DRY_RUN == 1 ]]; then
      info "(dry-run) systemctl enable --now $BIN_NAME.service"
    else
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
    fi

    step "Verify"
    if [[ $DRY_RUN == 1 ]]; then
      info "(dry-run) systemctl is-active $BIN_NAME.service"
    else
      sleep 2
      if systemctl is-active --quiet "$BIN_NAME.service"; then
        success "service is active — PID $(systemctl show -p MainPID --value "$BIN_NAME.service")"
      else
        warn "service is NOT active — recent logs:"
        journalctl -u "$BIN_NAME.service" -n 10 --no-pager | sed 's/^/    /' >&2
        fail "check: journalctl -u ${BIN_NAME}.service -f"
      fi
    fi
  else
    info "start skipped — enable later with: sudo systemctl enable --now ${BIN_NAME}"
  fi

  if command -v systemctl >/dev/null 2>&1 && command -v systemd-analyze >/dev/null 2>&1; then
    SCORE="$(systemd-analyze security "$BIN_NAME.service" 2>/dev/null | tail -1 || true)"
    info "hardening: ${SCORE:-n/a}"
  fi

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
  # Read the actually-installed user/root so we tear down what was installed,
  # not today's defaults.
  if [[ -f "$UNIT" ]]; then
    local u r
    u="$(sed -n 's/^User=//p' "$UNIT" 2>/dev/null || true)"
    r="$(sed -n 's/^Environment=LLM_PROXY_HOME=//p' "$UNIT" 2>/dev/null || true)"
    [[ -n "$u" ]] && SVC_USER="$u"
    [[ -n "$r" ]] && SVC_ROOT="$r"
  fi

  step "Artifacts found"
  [[ -f "$UNIT" ]]           && info "unit:         $UNIT"        || info "unit:         (not installed)"
  [[ -f "$INSTALL_BIN" ]]    && info "binary:       $INSTALL_BIN"  || info "binary:       (not installed)"
  [[ -d "$SVC_ROOT" ]]       && info "data root:    $SVC_ROOT"     || info "data root:    (not present)"
  id "$SVC_USER" &>/dev/null && info "service user: $SVC_USER"     || info "service user: (not present)"

  step "Stop & disable service"
  if [[ -f "$UNIT" ]]; then
    run systemctl stop "$BIN_NAME.service" 2>/dev/null || true
    run systemctl disable "$BIN_NAME.service" 2>/dev/null || true
    run systemctl reset-failed "$BIN_NAME.service" 2>/dev/null || true
    success "service stopped and disabled"
  else
    success "nothing to stop (skipping)"
  fi

  step "Remove unit file"
  if [[ -f "$UNIT" ]]; then
    run rm -f "$UNIT"
    run systemctl daemon-reload
    success "unit removed + daemon-reloaded"
  else
    success "no unit file (skipping)"
  fi

  step "Binary"
  if [[ -f "$INSTALL_BIN" ]] && { [[ $PURGE == 1 ]] || confirm "Remove $INSTALL_BIN? [y/N]"; }; then
    run rm -f "$INSTALL_BIN"
    success "binary removed"
  else
    info "binary kept"
  fi

  step "Data root (${SVC_ROOT})"
  if [[ ! -d "$SVC_ROOT" ]]; then
    success "not present (skipping)"
  elif [[ $PURGE != 1 ]] && confirm "Keep data at $SVC_ROOT (recommended — DB, secrets, master.key)?"; then
    info "data kept — reinstalling later picks it up automatically"
  elif [[ $PURGE == 1 ]] || confirm "Really delete everything under $SVC_ROOT? IRREVERSIBLE. [y/N]"; then
      # PURGE mode was already gated by the typed PURGE in purge_flow —
      # do not ask twice. Plain uninstall types DELETE here.
      if [[ $DRY_RUN == 1 ]]; then
        info "(dry-run) would require typing DELETE to proceed"
      elif [[ $PURGE == 1 ]]; then
        run rm -rf "$SVC_ROOT"
        success "data root erased (purge)"
      else
        local d=""
        if [[ $UI_BACKEND == ansi ]]; then
          read -r -p "  type DELETE to confirm: " d || true
        else
          d="$(ui_input "Type DELETE to confirm:" "")"
        fi
        if [[ "$d" == "DELETE" ]]; then
          run rm -rf "$SVC_ROOT"
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
    elif [[ $PURGE == 1 ]] || confirm "Remove system user $SVC_USER? [y/N]"; then
      run userdel "$SVC_USER"
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
      run rm -rf "$legacy_dir"
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
    local u r
    u="$(sed -n 's/^User=//p' "$UNIT" 2>/dev/null || true)"
    r="$(sed -n 's/^Environment=LLM_PROXY_HOME=//p' "$UNIT" 2>/dev/null || true)"
    [[ -n "$u" ]] && SVC_USER="$u"
    [[ -n "$r" ]] && SVC_ROOT="$r"
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

  if [[ $DRY_RUN == 1 ]]; then
    echo -e "${YELLOW}${BOLD}DRY RUN — full purge preview (nothing erased)${NC}"
    echo -e "$warning"
    PURGE=1 uninstall_flow
    return
  fi

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
    echo -e "\n${CYAN}${BOLD}[ $STEP ]${NC} ${BOLD}$1${NC}"
  else
    echo -e "\n>>> [ $STEP ] $1"
  fi
}

print_header() {
  if [[ $UI_BACKEND == ansi ]]; then
    echo -e "${CYAN}${BOLD}==================================================${NC}"
    echo -e "${CYAN}${BOLD}  llm-proxy — host setup${NC}"
    echo -e "${CYAN}${BOLD}==================================================${NC}"
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
    build)     run bash "$PRJ_ROOT/scripts/build.sh" ;;
    preview)   install_flow ;;
    *)
      case "$(ui_menu "What would you like to do?" \
        install           "Install — build binary + register systemd service" \
        register          "Register systemd only (binary already built)" \
        build             "Build binary only" \
        uninstall         "Uninstall — remove service (asks about data)" \
        purge             "Full purge — uninstall + erase EVERYTHING" \
        preview_install   "Preview install (dry-run)" \
        preview_uninstall "Preview uninstall (dry-run)" \
        quit              "Quit")" in
        install)           DO_BUILD=1; install_flow ;;
        register)          install_flow ;;
        build)             run bash "$PRJ_ROOT/scripts/build.sh" ;;
        uninstall)         if confirm "Uninstall the llm-proxy service? [y/N]"; then uninstall_flow; else warn "aborted"; fi ;;
        purge)             purge_flow ;;
        preview_install)   DRY_RUN=1; warn "DRY RUN — no changes will be made"; install_flow ;;
        preview_uninstall) DRY_RUN=1; warn "DRY RUN — no changes will be made"; uninstall_flow ;;
        *) exit 0 ;;
      esac
      ;;
  esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
