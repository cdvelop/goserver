package httpd

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// fakeAddr is a net.Addr carrying a fixed CIDR, for stubbing interfaceAddrs.
type fakeAddr struct{ s string }

func (f fakeAddr) Network() string { return "ip+net" }
func (f fakeAddr) String() string  { return f.s }

func stubInterfaceAddrs(t *testing.T, cidrs ...string) {
	t.Helper()
	prev := interfaceAddrs
	interfaceAddrs = func() ([]net.Addr, error) {
		out := make([]net.Addr, 0, len(cidrs))
		for _, c := range cidrs {
			ip, ipnet, err := net.ParseCIDR(c)
			if err != nil {
				t.Fatalf("bad test CIDR %q: %v", c, err)
			}
			ipnet.IP = ip
			out = append(out, ipnet)
		}
		return out, nil
	}
	t.Cleanup(func() { interfaceAddrs = prev })
}

func loadCert(t *testing.T, certFile string) *x509.Certificate {
	t.Helper()
	pemBytes, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatalf("reading cert: %v", err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("cert file is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing cert: %v", err)
	}
	return cert
}

func hasIP(ips []net.IP, want string) bool {
	w := net.ParseIP(want)
	for _, ip := range ips {
		if ip.Equal(w) {
			return true
		}
	}
	return false
}

// Test 7 — the development certificate carries localhost, both loopbacks, and
// every non-loopback IPv4 of the host, read from interfaceAddrs.
func TestDevCert_CoversLoopbackAndLAN(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubInterfaceAddrs(t, "192.168.1.50/24", "10.0.0.7/8", "127.0.0.1/8", "::1/128", "fe80::1/64")

	certFile, _, err := ensureDevCert(nil)
	if err != nil {
		t.Fatalf("ensureDevCert: %v", err)
	}
	cert := loadCert(t, certFile)

	foundLocalhost := false
	for _, n := range cert.DNSNames {
		if n == devCertHostname {
			foundLocalhost = true
		}
	}
	if !foundLocalhost {
		t.Errorf("DNSNames %v missing %q", cert.DNSNames, devCertHostname)
	}
	for _, want := range []string{"127.0.0.1", "::1", "192.168.1.50", "10.0.0.7"} {
		if !hasIP(cert.IPAddresses, want) {
			t.Errorf("IPAddresses %v missing %s", cert.IPAddresses, want)
		}
	}
	// Link-local IPv6 is not an IPv4 LAN address and must not be added.
	if hasIP(cert.IPAddresses, "fe80::1") {
		t.Errorf("IPAddresses unexpectedly contains link-local fe80::1")
	}
}

// Test 7 (cont.) — when the host's address set changes, the certificate is
// regenerated rather than kept for an address that is gone.
func TestDevCert_RegeneratesWhenAddressSetChanges(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	stubInterfaceAddrs(t, "192.168.1.50/24")
	certFile, _, err := ensureDevCert(nil)
	if err != nil {
		t.Fatalf("ensureDevCert #1: %v", err)
	}
	first := loadCert(t, certFile)
	if !hasIP(first.IPAddresses, "192.168.1.50") {
		t.Fatalf("first cert missing 192.168.1.50: %v", first.IPAddresses)
	}

	stubInterfaceAddrs(t, "192.168.9.9/24")
	if _, _, err := ensureDevCert(nil); err != nil {
		t.Fatalf("ensureDevCert #2: %v", err)
	}
	second := loadCert(t, certFile)

	if second.SerialNumber.Cmp(first.SerialNumber) == 0 {
		t.Error("certificate was not regenerated after the address set changed")
	}
	if hasIP(second.IPAddresses, "192.168.1.50") {
		t.Errorf("regenerated cert still names the old address 192.168.1.50: %v", second.IPAddresses)
	}
	if !hasIP(second.IPAddresses, "192.168.9.9") {
		t.Errorf("regenerated cert missing the new address 192.168.9.9: %v", second.IPAddresses)
	}
}

func TestDevCert_ReusesCertWhenAddressSetUnchanged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubInterfaceAddrs(t, "192.168.1.50/24")

	certFile, _, err := ensureDevCert(nil)
	if err != nil {
		t.Fatalf("ensureDevCert #1: %v", err)
	}
	first := loadCert(t, certFile)

	if _, _, err := ensureDevCert(nil); err != nil {
		t.Fatalf("ensureDevCert #2: %v", err)
	}
	second := loadCert(t, certFile)

	if first.SerialNumber.Cmp(second.SerialNumber) != 0 {
		t.Error("certificate was regenerated even though the address set did not change")
	}
}

// Test 6 — DevCertSPKI returns a stable 44-character base64 string for a fixed
// certificate.
func TestDevCertSPKI_StableBase64(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubInterfaceAddrs(t, "192.168.1.50/24")

	if _, _, err := ensureDevCert(nil); err != nil {
		t.Fatalf("ensureDevCert: %v", err)
	}

	first, err := DevCertSPKI()
	if err != nil {
		t.Fatalf("DevCertSPKI: %v", err)
	}
	if len(first) != 44 {
		t.Errorf("SPKI hash length = %d, want 44 (%q)", len(first), first)
	}
	second, err := DevCertSPKI()
	if err != nil {
		t.Fatalf("DevCertSPKI (again): %v", err)
	}
	if first != second {
		t.Errorf("DevCertSPKI not stable: %q != %q", first, second)
	}
}

// Test 8 (unit half) — DevCertDER returns bytes that parse as an X.509
// certificate.
func TestDevCertDER_IsParseableDER(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubInterfaceAddrs(t, "192.168.1.50/24")

	der, err := DevCertDER()
	if err != nil {
		t.Fatalf("DevCertDER: %v", err)
	}
	if _, err := x509.ParseCertificate(der); err != nil {
		t.Fatalf("DevCertDER did not return valid DER: %v", err)
	}
	// Sanity: it is the file ensureDevCert wrote.
	dir, _ := devCertDir()
	if _, err := os.Stat(filepath.Join(dir, devCertFilename)); err != nil {
		t.Fatalf("expected cert on disk: %v", err)
	}
}
