#!/usr/bin/env bash
# Report on one monitor-mode capture (2026-11-13 Wi-Fi investigation). Passive: it only reads a file.
#   ./analyse.sh <capture.pcap> [label]     needs tshark:  sudo apt install tshark
#   ./analyse.sh <fields.tsv>   [label]     re-runs the report from the table a previous run kept next to the pcap
# Prints to stdout; run.sh appends it to out-*/summary.txt. The pcap holds neighbours' MAC addresses: keep it private.
set -u
IN=${1:?usage: analyse.sh <capture.pcap|fields.tsv> [label]}; LABEL=${2:-$1}
if [ "${IN%.tsv}" != "$IN" ]; then
  TSV=$IN; PCAP=${IN%.tsv}
else
  PCAP=$IN; TSV=$IN.tsv
  command -v tshark >/dev/null || { echo "== $LABEL: tshark missing (sudo apt install tshark), then: ./analyse.sh $IN"; exit 0; }
  # columns: 1 time 2 subtype 3 type 4 transmitter 5 receiver 6 bssid 7 length 8 retry 9 signal 10 reason 11 rate 12 ssid 13 freq
  tshark -r "$PCAP" -T fields -E separator=/t -e frame.time_epoch -e wlan.fc.type_subtype -e wlan.fc.type -e wlan.ta -e wlan.ra \
    -e wlan.bssid -e frame.len -e wlan.fc.retry -e radiotap.dbm_antsignal -e wlan.fixed.reason_code -e radiotap.datarate \
    -e wlan.ssid -e radiotap.channel.freq >"$TSV" 2>"$PCAP.tshark.err"
fi
[ -s "$TSV" ] || { echo "== $LABEL: no frames"; exit 0; }
T0=$(head -1 "$TSV" | cut -f1)
echo; echo "================ $LABEL ================"

awk -F'\t' '
function nm(s) {
  if (s==0) return "assoc request"; if (s==1) return "assoc response"; if (s==2) return "reassoc request"; if (s==3) return "reassoc response"
  if (s==4) return "probe request"; if (s==5) return "probe response"; if (s==8) return "BEACON"; if (s==10) return "DISASSOCIATION"
  if (s==11) return "authentication"; if (s==12) return "DEAUTHENTICATION"; if (s==13) return "action"; if (s==14) return "action (no ack)"
  if (s==24) return "control: block-ack request"; if (s==25) return "control: block ack"; if (s==26) return "control: ps-poll"
  if (s==27) return "control: RTS"; if (s==28) return "control: CTS"; if (s==29) return "control: ACK"
  if (s==32) return "data"; if (s==36) return "data: null"; if (s==40) return "data: QoS"; if (s==44) return "data: QoS null"
  return "other (" s ")" }
{ n++; c[$2]++; if (!t0) t0=$1; t1=$1; if ($8=="1") { r++; if ($3=="2") dr++ } if ($3=="2") dn++ }
END { printf "frames %d in %.0f s (%.0f/s)\n", n, t1-t0, (t1>t0)?n/(t1-t0):n
      printf "retry flag: %.1f%% of all frames, %.1f%% of data frames (%d of %d)\n", n?100*r/n:0, dn?100*dr/dn:0, dr, dn
      for (k in c) printf "TYPE %7d  %s\n", c[k], nm(k) }' "$TSV" | sort -k1,1 -k2,2nr | sed 's/^TYPE /  /'

echo; echo "-- how much of the air this adapter can decode (frames with each radio header)"
if command -v tshark >/dev/null && [ -f "$PCAP" ]; then
  tshark -G fields 2>/dev/null >"$PCAP.fields"
  for f in radiotap.mcs radiotap.vht radiotap.he; do
    if grep -Fq "	$f	" "$PCAP.fields"; then
      echo "  $f: $(tshark -r "$PCAP" -Y "$f" -T fields -e frame.number 2>/dev/null | wc -l) frames"
    else echo "  $f: field not known to this tshark"; fi
  done
  echo "  (he = Wi-Fi 6. Zero beside an AX router and busy devices means this adapter cannot read Wi-Fi 6 frames.)"
fi

echo; echo "-- access points seen (beacons): count, BSSID, frequency MHz, mean signal dBm, name"
awk -F'\t' '$2==8 { k=$6 "\t" $13 "\t" $12; n[k]++; split($9,s,","); sg[k]+=s[1] }
  END { for (k in n) printf "%6d\t%s\t%d\n", n[k], k, sg[k]/n[k] }' "$TSV" | sort -rn | awk -F'\t' '{printf "  %6d  %s  %s MHz  %d dBm  [%s]\n", $1,$2,$3,$5,$4}' | head -40

echo; echo "-- devices (data frames): bytes up/down, frames, retry %, mean signal of its own frames, address kind"
awk -F'\t' '
function group(a,  d) { d=substr(a,2,1); return (d ~ /[13579bdfBDF]/) }
function locadm(a,  d) { d=substr(a,2,1); return (d ~ /[26aeAE]/) }
$3=="2" && $4!="" {
  down = ($4==$6); cl = down ? $5 : $4
  if (cl=="" || group(cl)) cl="(group/broadcast)"
  if (down) dn[cl]+=$7; else { up[cl]+=$7; split($9,s,","); sg[cl]+=s[1]; su[cl]++; if ($8=="1") rt[cl]++ }
  fr[cl]++; seen[cl]=1 }
END { for (c in seen) printf "%d\t%s\t%d\t%d\t%d\t%.0f\t%.0f\t%s\n", up[c]+dn[c], c, up[c], dn[c], fr[c], su[c]?100*rt[c]/su[c]:0, su[c]?sg[c]/su[c]:0, (c ~ /^\(/)?"":(locadm(c)?"private(random)":"hardware") }' "$TSV" |
  sort -rn | head -30 | awk -F'\t' '{printf "  %-17s up %9d  down %9d  frames %6d  retry %3d%%  signal %4d dBm  %s\n", $2,$3,$4,$5,$6,$7,$8}'

echo; echo "-- activity timeline of the 6 busiest devices: kB per 10 s bucket, first bucket = capture start ($T0)"
TOP=$(awk -F'\t' '$3=="2" && $4!="" { cl=($4==$6)?$5:$4; b[cl]+=$7 } END { for (c in b) print b[c] "\t" c }' "$TSV" | sort -rn | grep -v '^[0-9]*\s*\(ff:\|01:\|33:33\)' | head -6 | cut -f2 | tr '\n' ' ')
awk -F'\t' -v list="$TOP" -v t0="$T0" 'BEGIN { m=split(list, L, " "); for (i=1;i<=m;i++) want[L[i]]=1 }
  $3=="2" && $4!="" { cl=($4==$6)?$5:$4; if (cl in want) { b=int(($1-t0)/10); v[cl, b]+=$7; if (b>mx) mx=b } }
  END { for (i=1;i<=m;i++) { printf "  %-17s", L[i]; for (b=0;b<=mx;b++) printf " %d", v[L[i], b]/1024; printf "\n" } }' "$TSV"

echo; echo "-- connection events (assoc, reassoc, disassoc, auth, deauth) with reason; time = seconds from capture start"
awk -F'\t' -v t0="$T0" 'function nm(s) { if (s==0) return "assoc-req"; if (s==1) return "assoc-resp"; if (s==2) return "reassoc-req"; if (s==3) return "reassoc-resp"; if (s==10) return "DISASSOC"; if (s==11) return "auth"; return "DEAUTH" }
  $2==0||$2==1||$2==2||$2==3||$2==10||$2==11||$2==12 { printf "  %7.1f s  %-12s %s -> %s  bssid %s  reason %s  signal %s\n", $1-t0, nm($2), $4, $5, $6, ($10==""?"-":$10), $9 }' "$TSV" | head -150
echo "  (reason codes: 1 unspecified, 2 auth no longer valid, 3 sender is leaving, 4 inactivity, 7 class-3 frame from non-associated, 8 leaving BSS, 15 4-way handshake timeout)"

echo; echo "-- who asks for which networks (probe requests): count, device, name asked for (empty = any)"
awk -F'\t' '$2==4 { n[$4 "\t" $12]++ } END { for (k in n) printf "%d\t%s\n", n[k], k }' "$TSV" | sort -rn | head -25 | awk -F'\t' '{printf "  %5d  %-17s [%s]\n", $1,$2,$3}'

if command -v tshark >/dev/null && [ -f "$PCAP" ]; then
  echo; echo "-- what each access point announces (one beacon each): security, management-frame protection, WPS, load, country, standards"
  for b in $(awk -F'\t' '$2==8 {n[$6]++} END{for(k in n) print n[k], k}' "$TSV" | sort -rn | head -10 | cut -d' ' -f2); do
    echo "  BSSID $b"
    tshark -V -r "$PCAP" -Y "wlan.bssid==$b && wlan.fc.type_subtype==8" -c 1 2>/dev/null |
      grep -iE 'SSID:|Country Code|AKM|Cipher Suite|Pairwise|Management Frame Protection|RSN Capabilities|Wi-Fi Protected Setup|WPS|Config Methods|Device Name|Model|Manufacturer|Channel Utilization|Station Count|HE Capabilities|VHT Capabilities|HT Capabilities|Extended Capabilities|Vendor Specific' |
      sed 's/^ *//' | sort -u | cut -c1-110 | head -28 | sed 's/^/      /'
  done
fi
