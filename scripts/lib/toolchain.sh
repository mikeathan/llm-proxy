#!/usr/bin/env bash
# scripts/lib/toolchain.sh — locating the Go toolchain across environments.
#
# Sourced, never executed. Bash 3.2+.
#
# Both scripts/build.sh (running as the invoking user) and setup.sh (running as
# root but building as the invoking user via sudo) must find go even when it is
# not on PATH — under sudo, sudo's reset PATH hides a toolchain that lives in
# the user's login shell, and some users only have go via an interactive shell
# config that `bash -l` never reads. Keeping the location list here means the
# two callers cannot drift apart.

# shellcheck shell=bash

# find_go_bin [home] — print the path to a go binary found in common install
# locations, or return 1 when none is found. $1 overrides HOME so callers can
# locate another user's go under sudo.
find_go_bin() {
  local home="${1:-$HOME}" d
  for d in /usr/local/go/bin "$home/go/bin" "$home/.local/bin" /snap/bin /usr/lib/go/bin /opt/go/bin; do
    if [[ -x "$d/go" ]]; then printf '%s\n' "$d/go"; return 0; fi
  done
  return 1
}
