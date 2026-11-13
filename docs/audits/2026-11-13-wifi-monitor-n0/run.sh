#!/usr/bin/env bash
# Owner-run measurement for the Wi-Fi monitor-mode investigation (2026-11-13). Run on vertex (Linux) with the USB adapter plugged in:
#   sudo ./run.sh            (no arguments)
# It finds the USB adapter and your router's channel by itself, puts the adapter in monitor mode, listens (sends nothing), asks you to
# toggle a phone's Wi-Fi for 3 minutes, then restores the adapter. Paste back out-*/summary.txt (the .pcap files hold neighbours' MAC
# addresses: keep them private). Optional overrides: IFACE=<adapter> CHANNEL=<router channel> SECS=<seconds per channel>.
set -u
[ "$(id -u)" -eq 0 ] || { echo "run with: sudo ./run.sh"; exit 1; }
for c in iw ip tcpdump; do command -v "$c" >/dev/null || { echo "missing: $c  (sudo apt install iw tcpdump)"; exit 1; }; done

# The adapter: a Wi-Fi interface that sits on USB, unless told which. vertex's own card is never touched.
IF=${IFACE:-}
if [ -z "$IF" ]; then
  for d in /sys/class/net/*/wireless; do
    n=$(basename "$(dirname "$d")")
    readlink -f "/sys/class/net/$n/device" | grep -q '/usb' && { IF=$n; break; }
  done
fi
[ -n "$IF" ] || { echo "no USB Wi-Fi adapter found. Plug it in and check: ip -br link   (or run: IFACE=<name> sudo -E ./run.sh)"; exit 1; }
readlink -f "/sys/class/net/$IF/device" | grep -q '/usb' || { echo "refusing: $IF is not a USB adapter"; exit 1; }
echo "adapter: $IF"

# Your router's channel, read from the card vertex is connected through (no scan).
freq_to_ch() { local f=$1; if [ "$f" -eq 2484 ]; then echo 14; elif [ "$f" -lt 3000 ]; then echo $(( (f - 2407) / 5 )); else echo $(( (f - 5000) / 5 )); fi; }
RCH=${CHANNEL:-}
if [ -z "$RCH" ]; then
  for w in /sys/class/net/*/wireless; do
    n=$(basename "$(dirname "$w")"); [ "$n" = "$IF" ] && continue
    f=$(iw dev "$n" link 2>/dev/null | awk '/freq:/{print int($2)}'); [ -n "$f" ] && { RCH=$(freq_to_ch "$f"); break; }
  done
fi
[ -n "$RCH" ] && echo "your router's channel: $RCH" || echo "router channel not found (vertex not on Wi-Fi?): using common channels only"
SECS=${SECS:-30}
CHANNELS=("${RCH:-6}" 1 6 11 36 149); readarray -t CHANNELS < <(printf '%s\n' "${CHANNELS[@]}" | awk '!s[$0]++')

OUT=out-$(date +%Y%m%d-%H%M%S); mkdir -p "$OUT"; S="$OUT/summary.txt"
restore() { ip link set "$IF" down 2>/dev/null; iw dev "$IF" set type managed 2>/dev/null; ip link set "$IF" up 2>/dev/null
            nmcli dev set "$IF" managed yes 2>/dev/null; echo "adapter put back to normal mode"; }
trap restore EXIT
{ echo "== adapter"; lsusb; readlink "/sys/class/net/$IF/device/driver"; uname -r; iw dev "$IF" info
  iw phy "$(iw dev "$IF" info | awk '/wiphy/{print "phy"$2}')" info | sed -n '/Supported interface modes/,/Band 2/p;/Frequencies/,+25p' | head -60
} >"$S" 2>&1
nmcli dev set "$IF" managed no 2>/dev/null
ip link set "$IF" down && iw dev "$IF" set type monitor && ip link set "$IF" up || { echo "monitor mode FAILED (see $S)" | tee -a "$S"; exit 1; }
echo "monitor mode: ok" >>"$S"; echo "monitor mode: ok"

# summarise one capture file into the summary
summarise() { # $1 = text dump, $2 = title
  local T=$1
  { echo; echo "== $2"
    echo "frames: $(wc -l <"$T")  beacons: $(grep -c ' Beacon ' "$T")  probe req: $(grep -c 'Probe Request' "$T")  deauth/disassoc: $(grep -cE 'Deauthentication|Disassociation' "$T")  retry: $(grep -c 'Retry' "$T")  data: $(grep -cE 'Data|QoS' "$T")"
    echo "distinct BSSIDs (beacons): $(grep ' Beacon ' "$T" | grep -o 'BSSID:[0-9a-f:]*' | sort -u | wc -l)  distinct stations (SA): $(grep -o 'SA:[0-9a-f:]*' "$T" | sort -u | wc -l)"
    echo "signal dBm weakest / median / strongest: $(grep -o '\-[0-9]*dBm' "$T" | tr -d 'dBm-' | sort -n | awk '{a[NR]=$1}END{if(NR)print "-"a[NR]" / -"a[int((NR+1)/2)]" / -"a[1]; else print "none"}')"
  } >>"$S"
}

echo "surveying channels ${CHANNELS[*]} (${SECS}s each, listening only)..."
for ch in "${CHANNELS[@]}"; do
  iw dev "$IF" set channel "$ch" 2>>"$S" || { echo "channel $ch: not supported" >>"$S"; continue; }
  timeout "$SECS" tcpdump -i "$IF" -e -n -s 512 -w "$OUT/ch$ch.pcap" >/dev/null 2>"$OUT/ch$ch.err"
  tcpdump -e -n -r "$OUT/ch$ch.pcap" >"$OUT/ch$ch.txt" 2>/dev/null
  summarise "$OUT/ch$ch.txt" "channel $ch (${SECS}s)"
  { echo "SSIDs:"; grep ' Beacon ' "$OUT/ch$ch.txt" | grep -o 'Beacon ([^)]*)' | sort | uniq -c | sort -rn | head -15; } >>"$S"
done

# Event capture on the router's channel: the owner makes normal events happen.
EC=${RCH:-6}; ES=180
echo
echo "NEXT: a ${ES}-second capture on channel $EC. When you press Enter, then:"
echo "  - turn a phone's Wi-Fi OFF then ON,  - power-cycle a smart plug,  - forget and rejoin the network on one device."
read -r -p "Press Enter to start (or Ctrl-C to skip)... " _ </dev/tty || true
iw dev "$IF" set channel "$EC" && timeout "$ES" tcpdump -i "$IF" -e -n -s 512 -w "$OUT/events.pcap" >/dev/null 2>"$OUT/events.err"
E="$OUT/events.txt"; tcpdump -e -n -r "$OUT/events.pcap" >"$E" 2>/dev/null
summarise "$E" "events (channel $EC, ${ES}s)"
{ for t in 'Deauthentication' 'Disassociation' 'Authentication' 'Association Request' 'Association Response' 'Reassociation' 'Action' 'EAPOL'; do
    echo "-- $t: $(grep -c "$t" "$E") frames; samples:"; grep "$t" "$E" | head -3 | cut -c1-260
  done
  echo "-- deauth/disassoc reasons:"; grep -E 'Deauthentication|Disassociation' "$E" | grep -o ': [A-Za-z0-9 ,()-]*$' | sort | uniq -c | sort -rn | head
  echo "-- retry / QoS data samples:"; grep -E 'Retry|QoS' "$E" | head -3 | cut -c1-260
} >>"$S"
echo; echo "done. Paste this file back: $S"
