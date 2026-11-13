#!/usr/bin/env bash
# Owner-run measurement for the Wi-Fi monitor-mode investigation (2026-11-13). Run on vertex (Linux) with the USB adapter(s) plugged in:
#   sudo ./run.sh            (no arguments; install tshark first for the full report: sudo apt install tshark)
# It finds EVERY USB Wi-Fi adapter and your router's channel by itself, puts them in monitor mode, listens on each at the same time
# (sends nothing), then runs one guided capture where you make things happen and type a short note each time. Everything is restored on
# exit. Paste back out-*/report.txt. The .pcap files hold neighbours' MAC addresses: keep them private.
# Optional overrides: IFACES="wlxaaa wlxbbb"  CHANNEL=<router channel>  SECS=<seconds per channel, 30>  EVENT_SECS=<240>
set -u
[ "$(id -u)" -eq 0 ] || { echo "run with: sudo ./run.sh"; exit 1; }
HERE=$(cd "$(dirname "$0")" && pwd)
for c in iw ip tcpdump; do command -v "$c" >/dev/null || { echo "missing: $c  (sudo apt install iw tcpdump)"; exit 1; }; done
command -v tshark >/dev/null || echo "NOTE: tshark is not installed, so the report will be thin. Ctrl-C and run: sudo apt install tshark  (answer No to the non-root question)"

# Adapters: every Wi-Fi interface that sits on USB. vertex's own card is never touched.
IFS=' ' read -r -a ADAPTERS <<<"${IFACES:-}"
if [ ${#ADAPTERS[@]} -eq 0 ]; then
  for d in /sys/class/net/*/wireless; do
    n=$(basename "$(dirname "$d")")
    readlink -f "/sys/class/net/$n/device" | grep -q '/usb' && ADAPTERS+=("$n")
  done
fi
[ ${#ADAPTERS[@]} -gt 0 ] || { echo "no USB Wi-Fi adapter found. Plug it in and check: ip -br link"; exit 1; }
for a in "${ADAPTERS[@]}"; do readlink -f "/sys/class/net/$a/device" | grep -q '/usb' || { echo "refusing: $a is not a USB adapter"; exit 1; }; done
echo "adapters: ${ADAPTERS[*]}"

freq_to_ch() { local f=$1; if [ "$f" -eq 2484 ]; then echo 14; elif [ "$f" -lt 3000 ]; then echo $(( (f - 2407) / 5 )); else echo $(( (f - 5000) / 5 )); fi; }
RCH=${CHANNEL:-}
if [ -z "$RCH" ]; then
  for w in /sys/class/net/*/wireless; do
    n=$(basename "$(dirname "$w")"); case " ${ADAPTERS[*]} " in *" $n "*) continue;; esac
    f=$(iw dev "$n" link 2>/dev/null | awk '/freq:/{print int($2)}'); [ -n "$f" ] && { RCH=$(freq_to_ch "$f"); break; }
  done
fi
echo "your router's channel: ${RCH:-not found}"
SECS=${SECS:-30}; ES=${EVENT_SECS:-240}
OUT=out-$(date +%Y%m%d-%H%M%S); mkdir -p "$OUT"; S="$OUT/summary.txt"

restore() { for a in "${ADAPTERS[@]}"; do ip link set "$a" down 2>/dev/null; iw dev "$a" set type managed 2>/dev/null; ip link set "$a" up 2>/dev/null
              nmcli dev set "$a" managed yes 2>/dev/null; done; echo "adapters put back to normal mode"; }
trap restore EXIT

phy_of() { iw dev "$1" info | awk '/wiphy/{print "phy"$2}'; }
supported_channels() { iw phy "$(phy_of "$1")" info | grep -E '\* [0-9.]+ MHz \[[0-9]+\]' | grep -v 'disabled' | sed -n 's/.*\[\([0-9]*\)\].*/\1/p' | sort -un; }

{ echo "== host"; uname -r; date; iw reg get; rfkill list 2>/dev/null; echo; echo "== usb"; lsusb
  echo; echo "== kernel messages about the adapters"; dmesg 2>/dev/null | grep -iE 'rtw|mt76|mt79|rtl|wlx|usb.*wlan|cfg80211|regulatory' | tail -30
} >"$S" 2>&1

# Put each adapter in monitor mode and record what it can do.
GOOD=()
for a in "${ADAPTERS[@]}"; do
  mkdir -p "$OUT/$a"
  { echo; echo "== adapter $a"; readlink -f "/sys/class/net/$a/device/driver"; ethtool -i "$a" 2>/dev/null; iw dev "$a" info; supported_channels "$a" | tr '\n' ' '; echo
  } >>"$S" 2>&1
  iw phy "$(phy_of "$a")" info >"$OUT/$a/iw-phy-info.txt" 2>&1
  { echo "interface modes:"; sed -n '/Supported interface modes/,/Band 1/p' "$OUT/$a/iw-phy-info.txt" | head -14
    echo "wifi generations advertised:"; grep -ciE 'HE Iftypes|HE MAC|HE PHY' "$OUT/$a/iw-phy-info.txt" | sed 's/^/  HE (Wi-Fi 6) lines: /'
    grep -ciE 'VHT Capabilities' "$OUT/$a/iw-phy-info.txt" | sed 's/^/  VHT (Wi-Fi 5) lines: /'
    echo "bands:"; grep -E '^\s+Band [0-9]' "$OUT/$a/iw-phy-info.txt"; } >>"$S"
  nmcli dev set "$a" managed no 2>/dev/null
  if ip link set "$a" down && iw dev "$a" set type monitor && ip link set "$a" up; then echo "$a: monitor mode ok" | tee -a "$S"; GOOD+=("$a")
  else echo "$a: monitor mode FAILED" | tee -a "$S"; fi
done
[ ${#GOOD[@]} -gt 0 ] || { echo "no adapter would enter monitor mode (see $S)"; exit 1; }

# Per adapter: the channels it supports out of a wanted list, router channel first.
survey() { # $1 = interface
  local a=$1 sup ch want=()
  sup=" $(supported_channels "$a" | tr '\n' ' ') "
  for ch in ${RCH:-6} 1 6 11 36 52 100 149; do case "$sup" in *" $ch "*) printf '%s\n' "$ch";; esac; done | awk '!s[$0]++' >"$OUT/$a/channels.txt"
  while read -r ch; do
    iw dev "$a" set channel "$ch" 2>"$OUT/$a/ch$ch.err" || { echo "channel $ch: set failed" >>"$OUT/$a/survey.log"; continue; }
    sleep 1; echo "channel $ch: adapter reports: $(iw dev "$a" info | grep -E 'channel' | tr -s ' ')" >>"$OUT/$a/survey.log"
    timeout "$SECS" tcpdump -i "$a" -e -n -s 512 -w "$OUT/$a/ch$ch.pcap" >/dev/null 2>>"$OUT/$a/ch$ch.err"
    echo "channel $ch: $(tcpdump -n -r "$OUT/$a/ch$ch.pcap" 2>/dev/null | wc -l) frames in ${SECS}s" >>"$OUT/$a/survey.log"
  done <"$OUT/$a/channels.txt"
}
echo "surveying (${SECS}s per channel, every adapter at once, listening only)... this takes a few minutes"
for a in "${GOOD[@]}"; do survey "$a" & done; wait
for a in "${GOOD[@]}"; do { echo; echo "== survey $a (channels tried: $(tr '\n' ' ' <"$OUT/$a/channels.txt"))"; cat "$OUT/$a/survey.log"; } >>"$S"; done

# Event capture: every adapter at once, on the router's channel if it can, else on channel 6 (2.4 GHz).
echo
echo "NEXT: a ${ES}-second capture. When you press Enter, make things happen and, EACH TIME, type a short note and press Enter, e.g.:"
echo "    phone wifi off      phone wifi on      zigbee lamp 1 on      zigbee sensor off      forgot network on laptop"
echo "(the notes are time-stamped so I can line them up with the radio data; more varied events is better)"
read -r -p "Press Enter to start (Ctrl-C to skip)... " _ </dev/tty || true
echo ">>> CAPTURING for ${ES}s. Do something, then type what you did and press Enter (an empty Enter also marks the time)."
EPOCH0=$(date +%s)
for a in "${GOOD[@]}"; do
  sup=" $(supported_channels "$a" | tr '\n' ' ') "; ec=6; case "$sup" in *" ${RCH:-6} "*) ec=${RCH:-6};; esac
  echo "$ec" >"$OUT/$a/event-channel.txt"
  ( iw dev "$a" set channel "$ec" && timeout "$ES" tcpdump -i "$a" -e -n -s 512 -w "$OUT/$a/events.pcap" >/dev/null 2>"$OUT/$a/events.err" ) &
done
END=$(( $(date +%s) + ES )); : >"$OUT/marks.txt"
while [ "$(date +%s)" -lt "$END" ]; do
  if read -r -t 2 note </dev/tty; then echo "$(date +%s) $(date +%T) ${note:-(mark)}" >>"$OUT/marks.txt"; echo "   noted at $(date +%T)"; fi
done
wait
{ echo; echo "== your notes (epoch, clock, note). Capture started at epoch $EPOCH0"; cat "$OUT/marks.txt"; } >>"$S"

# Reports.
echo "analysing..."
for a in "${GOOD[@]}"; do
  for p in "$OUT/$a"/ch*.pcap; do [ -s "$p" ] && "$HERE/analyse.sh" "$p" "$a $(basename "$p" .pcap) (${SECS}s)" >>"$OUT/$a/details.txt" 2>&1; done
  "$HERE/analyse.sh" "$OUT/$a/events.pcap" "$a EVENTS on channel $(cat "$OUT/$a/event-channel.txt") (${ES}s)" >>"$S" 2>&1
done
cat "$S" "$OUT"/*/details.txt >"$OUT/report.txt" 2>/dev/null
echo; echo "done. Paste this file back: $OUT/report.txt   (long; if it is too long to paste, paste $S first)"
