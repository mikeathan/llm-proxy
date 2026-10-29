package netaudit

// remediations are the suggested fixes shown beside a check finding. They are fixed text written here, keyed by the
// check: nothing the device sent and nothing a model wrote reaches them, and nothing is changed on any device. The
// operator applies a fix and the next scan confirms it.
var remediations = map[string]string{
	CheckSMBv1:              "Turn off SMBv1 on the device or server; current Windows, Samba and NAS software uses SMB2 or SMB3.",
	CheckSMBSigning:         "Require SMB message signing in the device's file-sharing settings (on Samba, set \"server signing = mandatory\").",
	CheckFTPAnonymous:       "Turn off anonymous FTP sign-in, or stop the FTP service if nothing needs it; use SFTP where a transfer is needed.",
	CheckVNCNoAuth:          "Set a VNC password, or better, close the VNC port to the network and reach the screen through an SSH tunnel or VPN.",
	CheckRedisNoAuth:        "Set a password with \"requirepass\" and bind Redis to localhost, or firewall its port from the rest of the network.",
	CheckNFSExports:         "Limit each NFS export to the hosts that need it in the exports file, or stop NFS if it is unused.",
	CheckCertExpired:        "Renew the certificate on the device and install the new one; set a reminder before the next expiry.",
	CheckCertExpiring:       "Renew the certificate before it expires and install the new one on the device.",
	CheckSSHWeakAlgorithms:  "Remove the weak algorithms from the SSH server's configuration (Ciphers, MACs, KexAlgorithms, HostKeyAlgorithms) and update its software.",
	CheckKnownVulns:         "Update the software to the vendor's current release and check the vendor's notice for the listed numbers; a distribution may fix a flaw without changing the version, so if it is already fixed, accept the finding with a note.",
	CheckRDPNoNLA:           "Require network-level authentication in the Remote Desktop settings (on Windows: System Properties, Remote).",
	CheckSSHWeakHostKey:     "Remove the DSA and short RSA host keys from the SSH server and keep an Ed25519 or a 3072-bit RSA key, then restart the SSH service.",
	CheckTLSWeakKey:         "Issue a new certificate with an RSA key of at least 2048 bits (3072 is better) or an elliptic-curve key, and install it on the device.",
	CheckTLSWeakSignature:   "Issue a new certificate signed with SHA-256 or stronger, and install it on the device.",
	CheckMemcachedOpen:      "Make Memcached listen on localhost only, or firewall its port from the rest of the network; turn on SASL if other computers must reach it.",
	CheckMongoNoAuth:        "Turn on authentication in MongoDB's configuration, create the users first, and bind it to localhost or firewall its port.",
	CheckMQTTAnonymous:      "Turn off anonymous access on the MQTT broker and give each device its own user name and password.",
	CheckTelnetOpen:         "Turn off Telnet and use SSH instead; if a device cannot turn it off, update its firmware or keep it off the main network.",
	CheckDockerAPI:          "Stop Docker listening on the network (remove the tcp:// address from its hosts setting) and use SSH or a TLS-protected port for remote access.",
	CheckTLSWeakProtocol:    "Turn off SSL 3.0, TLS 1.0 and TLS 1.1 in the service's TLS settings and allow TLS 1.2 and 1.3 only; update the device's software or firmware if it cannot.",
	CheckTLSWeakCipher:      "Remove the weak ciphers from the service's TLS settings and use a current recommended cipher list; update the device's software or firmware if it cannot.",
	CheckSSHPasswordLogin:   "Set up key login for each account that needs SSH, then turn password login off in the SSH server's configuration (PasswordAuthentication no).",
	CheckUnexpectedPort:     "Close the port on the device if nothing needs it. If it should be open, add it to the device's expected ports so it stops being reported.",
	CheckExpectedPortClosed: "Check that the service on the device is running. If it was turned off on purpose, remove the port from the device's expected ports.",
}

// checkTitles name each check in a few plain words, for a heading over every place the problem was found. Like the fixes,
// they are fixed text written here.
var checkTitles = map[string]string{
	CheckSMBv1:              "SMBv1 enabled",
	CheckSMBSigning:         "SMB signing not required",
	CheckFTPAnonymous:       "Anonymous FTP",
	CheckVNCNoAuth:          "VNC with no password",
	CheckRedisNoAuth:        "Redis with no password",
	CheckNFSExports:         "NFS exports open to the network",
	CheckCertExpired:        "Expired certificate",
	CheckCertExpiring:       "Certificate about to expire",
	CheckSSHWeakAlgorithms:  "Weak SSH algorithms",
	CheckKnownVulns:         "Software with known flaws",
	CheckRDPNoNLA:           "Remote Desktop without network-level authentication",
	CheckSSHWeakHostKey:     "Weak SSH host key",
	CheckTLSWeakKey:         "Certificate with a short key",
	CheckTLSWeakSignature:   "Certificate with an outdated signature",
	CheckMemcachedOpen:      "Memcached open to the network",
	CheckMongoNoAuth:        "MongoDB without a login",
	CheckMQTTAnonymous:      "MQTT broker open to anyone",
	CheckTelnetOpen:         "Telnet open",
	CheckDockerAPI:          "Docker remote control open",
	CheckTLSWeakProtocol:    "Old TLS or SSL versions still accepted",
	CheckTLSWeakCipher:      "Weak TLS ciphers accepted",
	CheckSSHPasswordLogin:   "SSH accepts password login",
	CheckUnexpectedPort:     "Open port that is not expected",
	CheckExpectedPortClosed: "Expected port not answering",
}

// CheckTitle is the short name of a check, or "" for an open port or a check this version does not know.
func CheckTitle(check string) string {
	return checkTitles[check]
}

// RemediationFor is the suggested fix for a check, or "" for an open port or a check this version does not know.
func RemediationFor(check string) string {
	return remediations[check]
}
