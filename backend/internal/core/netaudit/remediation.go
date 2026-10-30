package netaudit

// remediations are the suggested fixes shown beside a check finding. They are fixed text written here, keyed by the
// check: nothing the device sent and nothing a model wrote reaches them, and nothing is changed on any device. The
// operator applies a fix and the next scan confirms it.
var remediations = map[string]string{
	CheckSMBv1:                "Turn off SMBv1 on the device or server; current Windows, Samba and NAS software uses SMB2 or SMB3.",
	CheckSMBSigning:           "Require SMB message signing in the device's file-sharing settings (on Samba, set \"server signing = mandatory\").",
	CheckFTPAnonymous:         "Turn off anonymous FTP sign-in, or stop the FTP service if nothing needs it; use SFTP where a transfer is needed.",
	CheckVNCNoAuth:            "Set a VNC password, or better, close the VNC port to the network and reach the screen through an SSH tunnel or VPN.",
	CheckRedisNoAuth:          "Set a password with \"requirepass\" and bind Redis to localhost, or firewall its port from the rest of the network.",
	CheckNFSExports:           "Limit each NFS export to the hosts that need it in the exports file, or stop NFS if it is unused.",
	CheckCertExpired:          "Renew the certificate on the device and install the new one; set a reminder before the next expiry.",
	CheckCertExpiring:         "Renew the certificate before it expires and install the new one on the device.",
	CheckSSHWeakAlgorithms:    "Remove the weak algorithms from the SSH server's configuration (Ciphers, MACs, KexAlgorithms, HostKeyAlgorithms) and update its software.",
	CheckKnownVulns:           "Update the software to the vendor's current release and check the vendor's notice for the listed numbers; a distribution may fix a flaw without changing the version, so if it is already fixed, accept the finding with a note.",
	CheckRDPNoNLA:             "Require network-level authentication in the Remote Desktop settings (on Windows: System Properties, Remote).",
	CheckSSHWeakHostKey:       "Remove the DSA and short RSA host keys from the SSH server and keep an Ed25519 or a 3072-bit RSA key, then restart the SSH service.",
	CheckTLSWeakKey:           "Issue a new certificate with an RSA key of at least 2048 bits (3072 is better) or an elliptic-curve key, and install it on the device.",
	CheckTLSWeakSignature:     "Issue a new certificate signed with SHA-256 or stronger, and install it on the device.",
	CheckMemcachedOpen:        "Make Memcached listen on localhost only, or firewall its port from the rest of the network; turn on SASL if other computers must reach it.",
	CheckMongoNoAuth:          "Turn on authentication in MongoDB's configuration, create the users first, and bind it to localhost or firewall its port.",
	CheckMQTTAnonymous:        "Turn off anonymous access on the MQTT broker and give each device its own user name and password.",
	CheckTelnetOpen:           "Turn off Telnet and use SSH instead; if a device cannot turn it off, update its firmware or keep it off the main network.",
	CheckDockerAPI:            "Stop Docker listening on the network (remove the tcp:// address from its hosts setting) and use SSH or a TLS-protected port for remote access.",
	CheckADBExposed:           "Turn off network debugging on the device (Developer options, or run \"adb usb\" on it), and keep it off any network you do not control.",
	CheckOpenProxy:            "Restrict the proxy to the computers that need it in its configuration, or stop it if nothing uses it; never leave one open to the whole network.",
	CheckElasticsearchExposed: "Turn on Elasticsearch's security so every request needs a login, and bind it to localhost or firewall port 9200 from the rest of the network.",
	CheckX11Exposed:           "Stop the display server listening on the network (start it with \"-nolisten tcp\") and use SSH forwarding if a remote screen is needed.",
	CheckRsyncExposed:         "Limit each rsync module to the hosts that need it and require a user and secrets file, or stop the rsync daemon if it is unused.",
	CheckKubeletExposed:       "Turn off anonymous access on the kubelet (anonymous.enabled: false, authorization mode Webhook) and firewall port 10250 to the cluster only.",
	CheckEtcdExposed:          "Make etcd listen on the cluster's own addresses only, require client certificates, and firewall port 2379 from everything else.",
	CheckNewDevice:            "Find out what the device is. If it is yours, approve it (or give it a name) in the device list; if you do not know it, change the Wi-Fi password and remove it from the router's list of connected devices.",
	CheckMACChanged:           "Check that the device that used to have this address is still on the network. If it is, restart it or give each device a fixed address in the router; if you cannot explain the change, treat the new device as unknown.",
	CheckGatewayChanged:       "Check whether the router was replaced, reset or its firmware updated; if so, confirm the new address in the device list. If not, disconnect from the network, check the devices on it, and change the router's password.",
	CheckDuplicateMAC:         "Find the device behind the address. A computer with several network connections or virtual machines does this; if the device is not that, unplug it and look for anything pretending to be other devices.",
	CheckDeviceMissing:        "Check that the device is on and connected. If you got rid of it, remove it from the device list.",
	CheckHiddenDevice:         "Find the device. A computer that is awake normally refuses connections; one that is silent on purpose may be hiding. Check it, or unplug it until you know what it is.",
	CheckHostKeyChanged:       "Find out whether the server was reinstalled or its SSH keys were regenerated. If so, accept the new keys on your computers (remove the old line from known_hosts); if nobody did that, treat the server as possibly impersonated and check the network.",
	CheckCertChanged:          "If the certificate was renewed on purpose, accept the finding. If you did not replace it, find out who did before trusting the service again.",
	CheckUPnPEnabled:          "Turn off UPnP in the router's settings unless something you use needs it; if it does, check the router's list of port mappings and remove any you did not expect.",
	CheckTLSWeakProtocol:      "Turn off SSL 3.0, TLS 1.0 and TLS 1.1 in the service's TLS settings and allow TLS 1.2 and 1.3 only; update the device's software or firmware if it cannot.",
	CheckTLSWeakCipher:        "Remove the weak ciphers from the service's TLS settings and use a current recommended cipher list; update the device's software or firmware if it cannot.",
	CheckSSHPasswordLogin:     "Set up key login for each account that needs SSH, then turn password login off in the SSH server's configuration (PasswordAuthentication no).",
	CheckUnexpectedPort:       "Close the port on the device if nothing needs it. If it should be open, add it to the device's expected ports so it stops being reported.",
	CheckExpectedPortClosed:   "Check that the service on the device is running. If it was turned off on purpose, remove the port from the device's expected ports.",
}

// checkTitles name each check in a few plain words, for a heading over every place the problem was found. Like the fixes,
// they are fixed text written here.
var checkTitles = map[string]string{
	CheckSMBv1:                "SMBv1 enabled",
	CheckSMBSigning:           "SMB signing not required",
	CheckFTPAnonymous:         "Anonymous FTP",
	CheckVNCNoAuth:            "VNC with no password",
	CheckRedisNoAuth:          "Redis with no password",
	CheckNFSExports:           "NFS exports open to the network",
	CheckCertExpired:          "Expired certificate",
	CheckCertExpiring:         "Certificate about to expire",
	CheckSSHWeakAlgorithms:    "Weak SSH algorithms",
	CheckKnownVulns:           "Software with known flaws",
	CheckRDPNoNLA:             "Remote Desktop without network-level authentication",
	CheckSSHWeakHostKey:       "Weak SSH host key",
	CheckTLSWeakKey:           "Certificate with a short key",
	CheckTLSWeakSignature:     "Certificate with an outdated signature",
	CheckMemcachedOpen:        "Memcached open to the network",
	CheckMongoNoAuth:          "MongoDB without a login",
	CheckMQTTAnonymous:        "MQTT broker open to anyone",
	CheckTelnetOpen:           "Telnet open",
	CheckDockerAPI:            "Docker remote control open",
	CheckADBExposed:           "Android debug bridge open",
	CheckOpenProxy:            "Open proxy",
	CheckElasticsearchExposed: "Elasticsearch open to the network",
	CheckX11Exposed:           "Graphical display open to the network",
	CheckRsyncExposed:         "rsync open to the network",
	CheckKubeletExposed:       "Kubernetes node control port open",
	CheckEtcdExposed:          "etcd open to the network",
	CheckNewDevice:            "New device on the network",
	CheckMACChanged:           "Address now used by a different device",
	CheckGatewayChanged:       "Router's hardware address changed",
	CheckDuplicateMAC:         "One device answering for several addresses",
	CheckDeviceMissing:        "Named device not seen for days",
	CheckHiddenDevice:         "Device that answers but stays silent",
	CheckHostKeyChanged:       "SSH host keys changed",
	CheckCertChanged:          "Certificate replaced",
	CheckUPnPEnabled:          "Router lets devices open internet ports (UPnP)",
	CheckTLSWeakProtocol:      "Old TLS or SSL versions still accepted",
	CheckTLSWeakCipher:        "Weak TLS ciphers accepted",
	CheckSSHPasswordLogin:     "SSH accepts password login",
	CheckUnexpectedPort:       "Open port that is not expected",
	CheckExpectedPortClosed:   "Expected port not answering",
}

// CheckTitle is the short name of a check, or "" for an open port or a check this version does not know.
func CheckTitle(check string) string {
	return checkTitles[check]
}

// RemediationFor is the suggested fix for a check, or "" for an open port or a check this version does not know.
func RemediationFor(check string) string {
	return remediations[check]
}
