package netaudit

// FixGuide is the longer form of a suggested fix: what to do in plain steps, commands to copy for the common platforms,
// what could go wrong, and how to undo it. It is fixed text this repository owns, keyed by the check; nothing a device
// sent and nothing a model wrote reaches it, and the app never runs a command in it. The operator applies the fix by
// hand and tells the app they did.
type FixGuide struct {
	Steps    []string       `json:"steps"`
	Commands []GuideCommand `json:"commands,omitempty"`
	// Risk says what the change can break, so the operator can choose the moment.
	Risk string `json:"risk"`
	// Undo says how to put things back.
	Undo string `json:"undo"`
}

// GuideCommand is a command to copy for one kind of system. It is shown, never run.
type GuideCommand struct {
	Platform string `json:"platform"`
	Command  string `json:"command"`
}

const (
	platformLinux   = "Linux"
	platformWindows = "Windows (PowerShell as administrator)"
)

// guides are written for current Debian and Ubuntu style systems, Samba, Windows 10 and later, and OpenSSH. Software
// differs, so each guide says to check before and after; the next scan is what confirms a fix.
var guides = map[string]FixGuide{
	CheckSMBv1: {
		Steps: []string{
			"Find which computers and devices still need SMBv1. Very old printers, scanners and Windows XP machines are the usual ones.",
			"On a Samba server, require SMB2 or newer in the [global] section of its configuration, then restart it.",
			"On Windows, remove the SMBv1 feature and restart the computer.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/samba/smb.conf /etc/samba/smb.conf.bak\n# in the [global] section of /etc/samba/smb.conf add:\n#   server min protocol = SMB2\ntestparm\nsudo systemctl restart smbd"},
			{platformWindows, "Disable-WindowsOptionalFeature -Online -FeatureName SMB1Protocol"},
		},
		Risk: "Devices that only speak SMBv1 lose access to the shares until they are updated or replaced.",
		Undo: "On Samba, put the saved smb.conf.bak back and restart smbd. On Windows, run Enable-WindowsOptionalFeature -Online -FeatureName SMB1Protocol and restart.",
	},
	CheckSMBSigning: {
		Steps: []string{
			"On a Samba server, require signing in the [global] section of its configuration, then restart it.",
			"On Windows, require signing for the server.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/samba/smb.conf /etc/samba/smb.conf.bak\n# in the [global] section of /etc/samba/smb.conf add:\n#   server signing = mandatory\ntestparm\nsudo systemctl restart smbd"},
			{platformWindows, "Set-SmbServerConfiguration -RequireSecuritySignature $true"},
		},
		Risk: "Clients that cannot sign messages are refused, and file transfers can be a little slower on weak hardware.",
		Undo: "On Samba, put the saved smb.conf.bak back and restart smbd. On Windows, run Set-SmbServerConfiguration -RequireSecuritySignature $false.",
	},
	CheckFTPAnonymous: {
		Steps: []string{
			"Decide whether anything needs the FTP service at all. If not, stop it and keep it off.",
			"If it is needed, turn off anonymous sign-in in the FTP server's configuration and restart it. Use SFTP for transfers where you can.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux + " (stop the service)", "sudo systemctl disable --now vsftpd"},
			{platformLinux + " (vsftpd, keep the service)", "sudo cp /etc/vsftpd.conf /etc/vsftpd.conf.bak\n# in /etc/vsftpd.conf set:\n#   anonymous_enable=NO\nsudo systemctl restart vsftpd"},
		},
		Risk: "Anything that downloads from the server without a login stops working.",
		Undo: "Run sudo systemctl enable --now vsftpd, or put the saved vsftpd.conf.bak back and restart the service.",
	},
	CheckVNCNoAuth: {
		Steps: []string{
			"Set a password on the VNC server, and make the VNC port unreachable from the rest of the network.",
			"Reach the screen through an SSH tunnel or a VPN instead of opening the port.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux + " (x11vnc: store a password)", "x11vnc -storepasswd"},
			{platformLinux + " (reach it through an SSH tunnel)", "ssh -L 5901:localhost:5901 user@the-vnc-host"},
		},
		Risk: "Open remote sessions are cut when the VNC server restarts, and anything that connects directly stops working.",
		Undo: "Restore the VNC server's previous settings and restart it. Keep a note of them before you change anything.",
	},
	CheckRedisNoAuth: {
		Steps: []string{
			"Choose a long random password and set it with requirepass in Redis's configuration.",
			"Make Redis listen only on the local machine unless another machine really needs it.",
			"Give every program that uses Redis the new password, then restart Redis.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "openssl rand -base64 32\nsudo cp /etc/redis/redis.conf /etc/redis/redis.conf.bak\n# in /etc/redis/redis.conf set:\n#   requirepass <the password you generated>\n#   bind 127.0.0.1 ::1\nsudo systemctl restart redis-server"},
		},
		Risk: "Programs that connect without the password fail until they are given it, and a restart drops open connections.",
		Undo: "Put the saved redis.conf.bak back and restart Redis.",
	},
	CheckNFSExports: {
		Steps: []string{
			"List which computers really need each exported folder.",
			"Edit the exports file so each folder is shared only with those computers, not with everyone, then reload it.",
			"If nothing needs NFS, stop the service instead.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/exports /etc/exports.bak\n# edit /etc/exports so each line names the hosts allowed, for example:\n#   /srv/share  host-name(rw,sync,no_subtree_check)\nsudo exportfs -ra"},
		},
		Risk: "Computers left off the list lose access to the share.",
		Undo: "Put the saved exports.bak back as /etc/exports and run sudo exportfs -ra.",
	},
	CheckCertExpired:  certificateGuide,
	CheckCertExpiring: certificateGuide,
	CheckSSHWeakAlgorithms: {
		Steps: []string{
			"Update the SSH server's software first; current releases already drop the weakest algorithms.",
			"List what the server offers now, then add a small configuration file that allows only the strong ones.",
			"Check the configuration, then reload the service. Keep your current SSH session open and try a second login before you close it.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo sshd -T | grep -E '^(ciphers|macs|kexalgorithms|hostkeyalgorithms)'\n# put the algorithms you want to keep in /etc/ssh/sshd_config.d/10-hardening.conf\nsudo sshd -t\nsudo systemctl reload ssh"},
		},
		Risk: "Old SSH clients that only know the removed algorithms can no longer log in. A mistake in the file can lock you out, which is why the current session stays open.",
		Undo: "Delete /etc/ssh/sshd_config.d/10-hardening.conf and reload the service (sudo systemctl reload ssh; on some systems the service is called sshd).",
	},
	CheckEndOfLife: {
		Steps: []string{
			"Find out which program the finding is about from the software listed under it, and which release you have.",
			"Look up the makers' current supported release, and whether your system's vendor still patches the version you run: many Linux distributions keep fixing an old number for years, and then there is nothing to do.",
			"If nothing supports it, take a snapshot or backup, move to a supported release, and restart the service.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux + " (Debian, Ubuntu)", "apt list --installed 2>/dev/null | grep -i <package>\napt policy <package>"},
		},
		Risk: "A newer release can change settings or behaviour that other programs rely on, so test it first where you can.",
		Undo: "Restore the snapshot or backup, or install the earlier version again (sudo apt install <package>=<version>).",
	},
	CheckKnownVulns: {
		Steps: []string{
			"Find out which program the finding is about from the software listed under it.",
			"Check whether the system's package manager has an update for it, and read the vendor's notice for the listed numbers. A distribution often fixes a flaw without changing the version number, so the notice says whether yours is already fixed.",
			"If it is already fixed, accept the finding with a note. If an update exists, take a snapshot or backup, apply it, and restart the service if asked.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux + " (Debian, Ubuntu)", "apt list --upgradable\napt changelog <package>\nsudo apt update && sudo apt install --only-upgrade <package>"},
		},
		Risk: "An update can restart the service and, now and then, change how the program behaves.",
		Undo: "Restore the snapshot or backup, or install the earlier version again (sudo apt install <package>=<version>).",
	},
	CheckRDPNoNLA: {
		Steps: []string{
			"Open System Properties on the Windows computer, go to Remote, and tick the box that allows connections only from computers running Remote Desktop with Network Level Authentication.",
			"Or run the command below.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformWindows, "(Get-WmiObject -Class Win32_TSGeneralSetting -Namespace root\\cimv2\\terminalservices -Filter \"TerminalName='RDP-tcp'\").SetUserAuthenticationRequired(1)"},
		},
		Risk: "Remote Desktop clients too old to support Network Level Authentication can no longer connect.",
		Undo: "Untick the box in System Properties, or run the same command with 0 instead of 1.",
	},
	CheckTLSWeakProtocol: {
		Steps: []string{
			"Find which program answers on that port (a web server, a router or NAS page, a mail server) and open its TLS settings.",
			"Allow TLS 1.2 and TLS 1.3 only. Turn off SSL 3.0, TLS 1.0 and TLS 1.1.",
			"Test the configuration, then restart or reload the service. If the device has no such setting, update its firmware or keep it off the main network.",
			"Run the deep check again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "# nginx: in the server or http block of its configuration set\n#   ssl_protocols TLSv1.2 TLSv1.3;\nsudo nginx -t && sudo systemctl reload nginx\n# Apache: in the TLS configuration set\n#   SSLProtocol -all +TLSv1.2 +TLSv1.3\nsudo apachectl configtest && sudo systemctl reload apache2"},
		},
		Risk: "Clients too old to speak TLS 1.2, such as Windows XP, very old phones and some embedded devices, can no longer connect.",
		Undo: "Put the saved configuration back (or add the old version to the protocol line again) and reload the service.",
	},
	CheckTLSWeakCipher: {
		Steps: []string{
			"Find which program answers on that port and open its TLS settings.",
			"Replace the cipher list with a current recommended one (the 'intermediate' list published by Mozilla is a good start). Remove RC4, 3DES, export-grade and anonymous ciphers.",
			"Test the configuration, then restart or reload the service.",
			"Run the deep check again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "# nginx: set ssl_ciphers to the Mozilla 'intermediate' list and\n#   ssl_prefer_server_ciphers off;\nsudo nginx -t && sudo systemctl reload nginx"},
		},
		Risk: "Clients that only support the removed ciphers can no longer connect.",
		Undo: "Put the saved configuration back and reload the service.",
	},
	CheckSSHPasswordLogin: {
		Steps: []string{
			"Put your public key on the account you use (ssh-copy-id), and confirm a key login works in a new window. Keep the old window open until you have.",
			"Set PasswordAuthentication to no (and KbdInteractiveAuthentication to no) in the SSH server's configuration.",
			"Test the configuration, then restart the SSH service.",
			"Run the deep check again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak\n# in /etc/ssh/sshd_config set:\n#   PasswordAuthentication no\n#   KbdInteractiveAuthentication no\n# files in /etc/ssh/sshd_config.d/ can override this: check them too\nsudo sshd -t && sudo systemctl restart ssh"},
		},
		Risk: "An account with no working key can no longer sign in over SSH, so test a key login first and keep a console or an open session as a way back.",
		Undo: "Put the saved sshd_config.bak back and restart the SSH service.",
	},
	CheckSSHWeakHostKey: {
		Steps: []string{
			"Check which host keys the server uses: look for HostKey lines in its configuration, and list the key files.",
			"Keep an Ed25519 key (create one if the server has none) and remove the DSA key and any RSA key shorter than 2048 bits.",
			"Test the configuration, then restart the SSH service.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak\nls /etc/ssh/ssh_host_*_key\n# create an Ed25519 key if it is missing:\n#   sudo ssh-keygen -t ed25519 -N \"\" -f /etc/ssh/ssh_host_ed25519_key\n# remove or comment out the HostKey lines for the dsa key and for short rsa keys in /etc/ssh/sshd_config\nsudo sshd -t && sudo systemctl restart ssh"},
		},
		Risk: "Computers that remember the old key will warn that the host key changed the next time they connect, and very old clients that only know DSA can no longer connect.",
		Undo: "Put the saved sshd_config.bak back and restart the SSH service.",
	},
	CheckTLSWeakKey:       weakCertificateGuide,
	CheckTLSWeakSignature: weakCertificateGuide,
	CheckMemcachedOpen: {
		Steps: []string{
			"Make Memcached listen on this computer only, unless other computers really need it.",
			"If they do, turn on SASL authentication and allow only those computers through the firewall.",
			"Restart the service and run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/memcached.conf /etc/memcached.conf.bak\n# in /etc/memcached.conf set:\n#   -l 127.0.0.1\nsudo systemctl restart memcached"},
		},
		Risk: "Programs on other computers that use this cache stop working until they are allowed again.",
		Undo: "Put the saved memcached.conf.bak back and restart the service.",
	},
	CheckMongoNoAuth: {
		Steps: []string{
			"Create an administrator user and a user for each program that uses the database, before turning authentication on.",
			"Turn on authorization in MongoDB's configuration and make it listen on this computer only, unless other computers need it.",
			"Restart MongoDB, update the programs to use their user names and passwords, and run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/mongod.conf /etc/mongod.conf.bak\n# in /etc/mongod.conf set:\n#   security:\n#     authorization: enabled\n#   net:\n#     bindIp: 127.0.0.1\nsudo systemctl restart mongod"},
		},
		Risk: "Programs that connect without a user name and password stop working until they are given one.",
		Undo: "Put the saved mongod.conf.bak back and restart MongoDB.",
	},
	CheckMQTTAnonymous: {
		Steps: []string{
			"Create a user name and password for each device and program that uses the broker.",
			"Turn off anonymous access in the broker's configuration and point it at the password file.",
			"Restart the broker, give each device its credentials, and run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux + " (Mosquitto)", "sudo mosquitto_passwd -c /etc/mosquitto/passwd first_user\n# in /etc/mosquitto/conf.d/auth.conf set:\n#   allow_anonymous false\n#   password_file /etc/mosquitto/passwd\nsudo systemctl restart mosquitto"},
		},
		Risk: "Devices that publish or subscribe without a login are cut off until they are given credentials.",
		Undo: "Remove auth.conf (or set allow_anonymous true) and restart the broker.",
	},
	CheckTelnetOpen: {
		Steps: []string{
			"Find the device. On a computer, stop the Telnet service and use SSH to log in instead.",
			"On a router, camera or other device, turn Telnet off in its settings page; if it has no such setting, update its firmware.",
			"If the device cannot turn it off, keep it off the main network or replace it.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux + " (stop the service)", "sudo systemctl disable --now telnet.socket   # or inetd or xinetd, depending on how Telnet was installed"},
		},
		Risk: "Anything that still logs in with Telnet, such as an old script or device, stops working.",
		Undo: "Run sudo systemctl enable --now telnet.socket, or switch the service back on in the device's settings.",
	},
	CheckUnexpectedPort: {
		Steps: []string{
			"Find out what is listening on that port: on a computer, the command below lists the program; on a router or other device, look in its services or settings page.",
			"If nothing needs it, stop the service or turn the setting off, then close the port in the device's firewall.",
			"If it should be open, add the port to the device's expected ports in this app so it is no longer reported.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo ss -ltnp   # lists each listening port and the program that owns it"},
		},
		Risk: "Anything that still connects to that port, such as another device or an old script, stops working.",
		Undo: "Start the service again or switch the setting back on, and remove the firewall rule if you added one.",
	},
	CheckExpectedPortClosed: {
		Steps: []string{
			"Check that the device is on and that the service behind the port is running; a restart of the service or the device often brings it back.",
			"If the service was turned off on purpose, remove the port from the device's expected ports in this app.",
			"Run the scan again to confirm.",
		},
		Risk: "Restarting a service interrupts anything using it at that moment.",
		Undo: "Stop the service again, or add the port back to the expected ports if you removed it.",
	},
	CheckDockerAPI: {
		Steps: []string{
			"Find how Docker was told to listen on a network port: look for a tcp:// address in its service file or in daemon.json.",
			"Remove that address so Docker listens on its local socket only.",
			"For remote access use SSH (docker -H ssh://user@host) or the TLS-protected port 2376 with certificates.",
			"Restart Docker and run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo ss -ltnp | grep 2375\nsudo cp /etc/docker/daemon.json /etc/docker/daemon.json.bak\n# remove the tcp:// entry from \"hosts\" in /etc/docker/daemon.json, and any -H tcp:// in the docker service file\nsudo systemctl restart docker"},
		},
		Risk: "Restarting Docker stops running containers unless live restore is on, and tools that used the open port lose access.",
		Undo: "Put the saved daemon.json.bak back and restart Docker.",
	},
	CheckADBExposed: {
		Steps: []string{
			"Find the device. Android phones, TV boxes and tablets open this port when network debugging is on.",
			"On the device, turn off USB debugging and wireless debugging in Developer options, or restart it, which closes the port on most versions.",
			"If the device is a TV box or streamer that comes with it open, keep it on a separate guest network or replace it; this port is how many botnets take such devices over.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux + " (from a computer with adb)", "adb connect DEVICE_ADDRESS:5555   # replace with the device's address\nadb usb   # makes the device listen on USB only"},
		},
		Risk: "A program or developer tool that used the network connection to the device stops working.",
		Undo: "Turn wireless debugging back on in Developer options.",
	},
	CheckOpenProxy: {
		Steps: []string{
			"Find which program is the proxy: on a computer the command below shows it; on a router or other device, look for a proxy or VPN setting.",
			"Restrict it to the computers that need it (for Squid, an http_access allow line for those addresses only, then http_access deny all), or stop it if nothing uses it.",
			"A proxy that you did not set up is a sign of a compromised device: change its passwords and update or reset it.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo ss -ltnp | grep -E ':(1080|3128)'   # shows the program that owns the port\nsudo systemctl disable --now squid   # if it is Squid and unused"},
		},
		Risk: "Anything configured to use the proxy loses its connection until it is pointed elsewhere.",
		Undo: "Start the service again and restore its saved configuration.",
	},
	CheckElasticsearchExposed: {
		Steps: []string{
			"Check whether Elasticsearch needs a login: a request to it should answer 401 without credentials.",
			"Turn on security in elasticsearch.yml and set the built-in users' passwords.",
			"Bind it to localhost or the one address that needs it (network.host), and firewall port 9200 from the rest of the network.",
			"Restart it and run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/elasticsearch/elasticsearch.yml /etc/elasticsearch/elasticsearch.yml.bak\n# in elasticsearch.yml set:\n#   xpack.security.enabled: true\n#   network.host: 127.0.0.1\nsudo systemctl restart elasticsearch"},
		},
		Risk: "Programs that sent requests without a login are refused until they are given credentials; binding to localhost cuts off other computers.",
		Undo: "Put the saved elasticsearch.yml.bak back and restart the service.",
	},
	CheckX11Exposed: {
		Steps: []string{
			"Find the computer. Its display server was started allowing network connections.",
			"Start it with network connections off (\"-nolisten tcp\"); most current desktops already do, so this is usually an old or custom setting.",
			"For a remote program window use SSH forwarding (ssh -X) instead.",
			"Restart the display server and run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo ss -ltnp | grep ':6000'   # shows which program listens\n# in the display manager's configuration remove any \"-listen tcp\" and keep \"-nolisten tcp\""},
		},
		Risk: "Restarting the display server closes every open window on that computer.",
		Undo: "Put the saved display manager configuration back and restart it.",
	},
	CheckRsyncExposed: {
		Steps: []string{
			"Open the rsync daemon's configuration and look at each module (a name in square brackets).",
			"Limit each module to the hosts that need it (hosts allow) and require a user and a secrets file (auth users, secrets file); make the module read only unless writing is needed.",
			"If nothing uses the rsync daemon, stop it and use rsync over SSH instead.",
			"Restart it and run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /etc/rsyncd.conf /etc/rsyncd.conf.bak\n# in each module add: hosts allow = ALLOWED_ADDRESS, auth users = a_user, secrets file = /etc/rsyncd.secrets\nsudo systemctl restart rsync"},
		},
		Risk: "Backups and syncs that used the module without a login fail until they are given credentials.",
		Undo: "Put the saved rsyncd.conf.bak back and restart the service.",
	},
	CheckKubeletExposed: {
		Steps: []string{
			"Check the node's kubelet configuration for anonymous access and the authorization mode.",
			"Set anonymous authentication off and the authorization mode to Webhook, then restart the kubelet.",
			"Firewall port 10250 so that only the cluster's own control plane can reach it.",
			"Run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo cp /var/lib/kubelet/config.yaml /var/lib/kubelet/config.yaml.bak\n# in config.yaml set: authentication.anonymous.enabled: false and authorization.mode: Webhook\nsudo systemctl restart kubelet"},
		},
		Risk: "Tools that call the kubelet without credentials stop working; a wrong setting can stop the node serving pods.",
		Undo: "Put the saved config.yaml.bak back and restart the kubelet.",
	},
	CheckEtcdExposed: {
		Steps: []string{
			"Find how etcd was told to listen: look for listen-client-urls in its configuration.",
			"Set it to the cluster's own addresses, turn on client certificate checks (client-cert-auth), and firewall port 2379 from everything else.",
			"Restart etcd one member at a time and run the scan again to confirm.",
		},
		Commands: []GuideCommand{
			{platformLinux, "sudo ss -ltnp | grep ':2379'\n# in the etcd configuration set listen-client-urls to the cluster addresses only, and client-cert-auth: true"},
		},
		Risk: "A cluster whose members or clients cannot reach etcd stops working; change one member at a time.",
		Undo: "Put the saved etcd configuration back and restart that member.",
	},
	CheckNewDevice: {
		Steps: []string{
			"Open the device list and look at the address and the first-seen date. Compare it with the devices in the router's list of connected devices; the router shows names for most of them.",
			"If it is a device of yours or a guest's, approve it (or give it a name) so it stops being reported.",
			"If you do not know it, remove it from the router's list or block it, change the Wi-Fi password, and reconnect only your own devices.",
			"Run the audit again to confirm.",
		},
		Risk: "Changing the Wi-Fi password disconnects every device until it is given the new one.",
		Undo: "Approve the device in the list again, or put the old Wi-Fi password back.",
	},
	CheckMACChanged: {
		Steps: []string{
			"Find out which device had the address before: it is the one you approved. Check that it is on and where it is connected.",
			"If it simply reconnected and the router gave its address to another device, that is harmless: approve the new device so it stops being reported.",
			"If you cannot explain it, treat the new device as unknown (see \"New device on the network\"). Giving your important devices fixed addresses in the router stops addresses from changing hands.",
		},
		Risk: "A fixed address that is also in use by something else causes a conflict until one of them is changed.",
		Undo: "Remove the fixed address in the router and let it hand out addresses again.",
	},
	CheckGatewayChanged: {
		Steps: []string{
			"Think about what changed: a new or reset router, a firmware update, or a router swapped by your provider all give it a new hardware address.",
			"If that happened, confirm the new address in the device list so it becomes the one on record.",
			"If nothing changed, assume something is pretending to be the router. Disconnect your own devices from the network, look for an unknown device in the router's list, and change the router's password and the Wi-Fi password.",
		},
		Risk: "Disconnecting devices interrupts everything on the network while you check.",
		Undo: "Reconnect the devices once the network has been checked.",
	},
	CheckDuplicateMAC: {
		Steps: []string{
			"Find the device that owns the hardware address in the device list; it shows every address it answers for.",
			"A server with several addresses, a computer running virtual machines or a device on both wired and Wi-Fi networks does this on purpose; if that is what it is, leave it.",
			"Otherwise something is answering for addresses that are not its own. Unplug it, and check the other devices on the network.",
		},
		Risk: "Unplugging a device interrupts whatever it does.",
		Undo: "Plug it back in.",
	},
	CheckDeviceMissing: {
		Steps: []string{
			"Check that the device is switched on and connected to the network.",
			"A device that has stopped answering because it failed or was thrown out should be removed from the device list so it stops being reported.",
		},
		Risk: "None: removing a device from the list does not touch the device.",
		Undo: "A device that comes back appears again as a new device, and can be approved or named again.",
	},
	CheckHiddenDevice: {
		Steps: []string{
			"Find the device behind the address with the router's list of connected devices.",
			"Many sleeping devices answer the network's address lookup and nothing else; if it is one of yours, give the device a name on the Devices card (a named device is not reported again) or give it a fixed address.",
			"A device you do not recognise that answers nothing at all is worth unplugging until you know what it is.",
		},
		Risk: "Unplugging a device interrupts whatever it does.",
		Undo: "Plug it back in.",
	},
	CheckHostKeyChanged: {
		Steps: []string{
			"Ask whether anyone reinstalled the server, restored it from a backup, or regenerated its SSH keys. Those are the usual reasons.",
			"If it was expected, remove the old entry on your computers so they trust the new keys, and accept the finding here.",
			"If nobody did that, do not log in with a password to it. Check the server from its own console, and look for an unknown device on the network.",
		},
		Commands: []GuideCommand{
			{platformLinux + " (on your own computer)", "ssh-keygen -R SERVER_NAME_OR_ADDRESS   # forgets the old key; the next login asks about the new one"},
		},
		Risk: "Trusting the new keys without checking would also trust an impersonator.",
		Undo: "Nothing is changed on the server; the old entry is only forgotten on your own computer.",
	},
	CheckCertChanged: {
		Steps: []string{
			"Find out whether the certificate was renewed on purpose. Many devices and web servers renew on a schedule.",
			"If it was, accept the finding; the next change will be reported again.",
			"If you did not expect a change, look at who issued the new certificate in a browser, and check the device and the network before trusting it.",
		},
		Risk: "Accepting a certificate nobody renewed would hide a service being replaced.",
		Undo: "Take the acceptance back and the finding returns.",
	},
	CheckUPnPEnabled: {
		Steps: []string{
			"Open the router's settings page (for most routers, its address in a browser) and find the UPnP switch. On many ASUS routers it is under WAN, Internet Connection, \"Enable UPnP\"; other brands put it under Advanced or NAT.",
			"Check whether anything you use needs it: some games consoles, video calls and torrent programs ask the router for ports through UPnP. If you are unsure, turn it off and see whether anything stops working.",
			"If you keep it on, look at the router's list of UPnP port mappings (often next to the switch) and remove any you do not recognise.",
			"Run the audit again to confirm.",
		},
		Risk: "Programs that relied on UPnP to open a port stop being reachable from outside until the port is forwarded by hand.",
		Undo: "Turn the UPnP switch back on in the router's settings.",
	},
	CheckDecoyTouched: {
		Steps: []string{
			"Find the device with the address shown, in the router's list of connected devices or in the device list here.",
			"Ask who uses it. A network scanner app on a phone, a laptop running a security tool or a smart-home hub scanning for devices will touch a decoy; so will malware looking for services to attack.",
			"If it is yours and expected, accept the finding. If you do not know what it is, disconnect it from the network, change the Wi-Fi password, and scan it with a security tool before it comes back.",
		},
		Risk: "Disconnecting a device interrupts whatever it does; changing the Wi-Fi password disconnects every device until it has the new one.",
		Undo: "Reconnect the device, or put the old Wi-Fi password back.",
	},
	CheckUPnPForward: {
		Steps: []string{
			"Open the router's settings page and find the list of UPnP port mappings (on many ASUS routers: WAN, Internet Connection, or the UPnP table next to the switch). It shows which device asked for which port.",
			"Find the program on that device that wants the port open: a game, a torrent or video program, a camera, or a home server. If you do not use it from outside your home, close it or turn its UPnP option off.",
			"Remove the mapping in the router and turn UPnP off if nothing else needs it. If you do need the service from outside, use a VPN to your home instead of an open port.",
			"Run the audit again to confirm.",
		},
		Risk: "Whatever relied on the open port stops being reachable from outside until it is forwarded by hand or reached through a VPN.",
		Undo: "Turn UPnP back on, or add the port forward in the router by hand.",
	},
}

// weakCertificateGuide serves both weak-certificate checks: the fix is a new certificate.
var weakCertificateGuide = FixGuide{
	Steps: []string{
		"Find which service or device uses the certificate. It is usually a web server, or a device's own web page.",
		"On a device, use its settings page to create a new certificate. On a server, issue a new one: from your certificate authority, or with the command below for a self-signed certificate.",
		"Install the new certificate (with its full chain) where the service reads it and reload the service.",
		"Run the scan again to confirm.",
	},
	Commands: []GuideCommand{
		{platformLinux + " (new self-signed certificate)", "openssl req -x509 -sha256 -newkey rsa:3072 -nodes -days 365 -keyout new-key.pem -out new-cert.pem -subj \"/CN=name-of-the-service\""},
	},
	Risk: "A certificate for the wrong name, or without its chain, makes browsers and programs refuse the service; programs that remembered the old certificate must be told about the new one.",
	Undo: "Put the previous certificate and key files back where the service reads them and reload it. Keep a copy before you replace them.",
}

// certificateGuide serves both certificate checks: the fix is the same, only the urgency differs.
var certificateGuide = FixGuide{
	Steps: []string{
		"Find which service uses the certificate. It is usually a web server, a mail server or a device's own web page.",
		"Renew it. If the system uses Let's Encrypt through certbot, renew with the command below; otherwise get a new certificate from the authority that issued the old one.",
		"Install the new certificate (with its full chain) where the service reads it and reload the service.",
		"Run the scan again to confirm, and set a reminder for the next expiry.",
	},
	Commands: []GuideCommand{
		{platformLinux + " (certbot)", "sudo certbot renew\nsudo systemctl reload nginx   # or apache2, or the service that uses the certificate"},
	},
	Risk: "A certificate installed without its chain, or for the wrong name, makes browsers and programs refuse the service.",
	Undo: "Put the previous certificate files back where the service reads them and reload it. Keep a copy before you replace them.",
}

// GuideFor returns the fix guide of a check, and false for an open port or a check this version does not know.
func GuideFor(check string) (FixGuide, bool) {
	g, ok := guides[check]
	return g, ok
}
