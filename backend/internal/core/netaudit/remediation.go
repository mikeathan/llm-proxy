package netaudit

// remediations are the suggested fixes shown beside a check finding. They are fixed text written here, keyed by the
// check: nothing the device sent and nothing a model wrote reaches them, and nothing is changed on any device. The
// operator applies a fix and the next scan confirms it.
var remediations = map[string]string{
	CheckSMBv1:             "Turn off SMBv1 on the device or server; current Windows, Samba and NAS software uses SMB2 or SMB3.",
	CheckSMBSigning:        "Require SMB message signing in the device's file-sharing settings (on Samba, set \"server signing = mandatory\").",
	CheckFTPAnonymous:      "Turn off anonymous FTP sign-in, or stop the FTP service if nothing needs it; use SFTP where a transfer is needed.",
	CheckVNCNoAuth:         "Set a VNC password, or better, close the VNC port to the network and reach the screen through an SSH tunnel or VPN.",
	CheckRedisNoAuth:       "Set a password with \"requirepass\" and bind Redis to localhost, or firewall its port from the rest of the network.",
	CheckNFSExports:        "Limit each NFS export to the hosts that need it in the exports file, or stop NFS if it is unused.",
	CheckCertExpired:       "Renew the certificate on the device and install the new one; set a reminder before the next expiry.",
	CheckCertExpiring:      "Renew the certificate before it expires and install the new one on the device.",
	CheckSSHWeakAlgorithms: "Remove the weak algorithms from the SSH server's configuration (Ciphers, MACs, KexAlgorithms, HostKeyAlgorithms) and update its software.",
	CheckKnownVulns:        "Update the software to the vendor's current release and check the vendor's notice for the listed numbers; a distribution may fix a flaw without changing the version, so if it is already fixed, accept the finding with a note.",
	CheckRDPNoNLA:          "Require network-level authentication in the Remote Desktop settings (on Windows: System Properties, Remote).",
}

// RemediationFor is the suggested fix for a check, or "" for an open port or a check this version does not know.
func RemediationFor(check string) string {
	return remediations[check]
}
