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
