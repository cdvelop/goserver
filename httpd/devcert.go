package httpd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/smallstep/truststore"
)

const (
	devCertOrg      = "WebTyp Dev CA"
	devCertHostname = "localhost"
	devCertIP4Loop  = "127.0.0.1"
	devCertIP6Loop  = "::1"

	devCertFilename  = "localhost.crt"
	devKeyFilename   = "localhost.key"
	devSANsFilename  = "localhost.sans" // records the SANs the cert on disk was built with
	pemTypeCert      = "CERTIFICATE"
	pemTypeECPrivate = "EC PRIVATE KEY"

	devCertDirPerm  os.FileMode = 0o755
	devKeyFilePerm  os.FileMode = 0o600
	devSANsFilePerm os.FileMode = 0o644
	devCertValidFor             = 365 * 24 * time.Hour

	// EnvSkipTruststore, when set to any non-empty value, stops the dev
	// certificate from being installed into the OS trust store. The server
	// still generates and serves the certificate (and CAPath still works); only
	// the root-requiring system-trust step — which shells out to `sudo` on
	// Linux and prompts — is skipped. Test runners and CI set this.
	EnvSkipTruststore = "WEBTYP_DEVCERT_SKIP_TRUSTSTORE"
)

// interfaceAddrs is net.InterfaceAddrs, indirected so tests can control the set
// of host addresses the development certificate is issued for.
var interfaceAddrs = net.InterfaceAddrs

// devCertDir is where the development certificate, key and SAN record live.
func devCertDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".webtyp", "httpd", "certs"), nil
}

// lanIPs returns every non-loopback IPv4 address of the host's active
// interfaces. A phone on the LAN opens https://192.168.x.x:<port>, and a
// certificate that does not name that address is rejected outright by Safari on
// iOS.
func lanIPs() []net.IP {
	var out []net.IP
	addrs, err := interfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip == nil || ip.IsLoopback() {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			out = append(out, v4)
		}
	}
	return out
}

// desiredSANs returns the DNS name and the IP addresses the development
// certificate must carry, and a stable string form of that set for change
// detection.
func desiredSANs() (dns []string, ips []net.IP, fingerprint string) {
	dns = []string{devCertHostname}
	ips = []net.IP{net.ParseIP(devCertIP4Loop), net.ParseIP(devCertIP6Loop)}
	ips = append(ips, lanIPs()...)

	parts := append([]string{}, dns...)
	for _, ip := range ips {
		parts = append(parts, ip.String())
	}
	sort.Strings(parts)
	return dns, ips, strings.Join(parts, ",")
}

// ensureDevCert returns paths to the development certificate and key, generating
// them on first use and regenerating them whenever the host's address set has
// changed since the cert on disk was written. logf, when non-nil, receives a
// best-effort warning if the CA cannot be installed in the OS truststore.
func ensureDevCert(logf func(...any)) (certFile, keyFile string, err error) {
	dir, err := devCertDir()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, devCertDirPerm); err != nil {
		return "", "", err
	}

	certFile = filepath.Join(dir, devCertFilename)
	keyFile = filepath.Join(dir, devKeyFilename)
	sansFile := filepath.Join(dir, devSANsFilename)

	dnsNames, ipAddrs, fingerprint := desiredSANs()

	if fresh(certFile, keyFile, sansFile, fingerprint) {
		return certFile, keyFile, nil
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return "", "", err
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{devCertOrg}},
		NotBefore:             now,
		NotAfter:              now.Add(devCertValidFor),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddrs,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	if err := writePEM(certFile, pemTypeCert, derBytes, devSANsFilePerm); err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	if err := writePEM(keyFile, pemTypeECPrivate, keyDER, devKeyFilePerm); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(sansFile, []byte(fingerprint), devSANsFilePerm); err != nil {
		return "", "", err
	}

	// Install the CA in the OS truststore (best effort — a phone still needs to
	// install it explicitly via CAPath). Skipped when EnvSkipTruststore is set,
	// so a test run or CI job never triggers the root prompt.
	if os.Getenv(EnvSkipTruststore) == "" {
		if cert, perr := x509.ParseCertificate(derBytes); perr == nil {
			if ierr := truststore.Install(cert); ierr != nil && logf != nil {
				logf("Warning: failed to install dev certificate in truststore (browsers may show warning):", ierr)
			}
		}
	}

	return certFile, keyFile, nil
}

// fresh reports whether the cert, key and SAN record on disk are all present and
// the recorded SAN set still matches the host's current addresses.
func fresh(certFile, keyFile, sansFile, fingerprint string) bool {
	for _, f := range []string{certFile, keyFile} {
		if _, err := os.Stat(f); err != nil {
			return false
		}
	}
	recorded, err := os.ReadFile(sansFile)
	if err != nil {
		return false
	}
	return string(recorded) == fingerprint
}

func writePEM(path, blockType string, der []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: der})
}

// getOrCreateDevCert is the listen path's entry point; it routes the truststore
// warning through the server's logger.
func (s *Server) getOrCreateDevCert() (string, string, error) {
	return ensureDevCert(s.log)
}

// DevCertFiles returns paths to the development certificate and key, creating
// and truststore-installing them on first use. It is the same artifact the
// DevTLS listen path serves, exposed for callers that manage their own
// *http.Server (the internal dev strategy in webtyp.com/server).
func DevCertFiles() (certFile, keyFile string, err error) {
	return ensureDevCert(nil)
}

// DevCertDER returns the development certificate, DER-encoded, generating it if
// necessary. It is what CAPath serves to a device on the LAN.
func DevCertDER() ([]byte, error) {
	certFile, _, err := ensureDevCert(nil)
	if err != nil {
		return nil, err
	}
	pemBytes, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errDevCertDecode
	}
	return block.Bytes, nil
}
