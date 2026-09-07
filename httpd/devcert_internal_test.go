package httpd

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webtyp.com/router"
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

// DevCA returns bytes that parse as an X.509 CA certificate.
func TestDevCA_IsParseableDER(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubInterfaceAddrs(t, "192.168.1.50/24")

	der, err := DevCA()
	if err != nil {
		t.Fatalf("DevCA: %v", err)
	}
	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("DevCA did not return valid DER: %v", err)
	}
	if !caCert.IsCA {
		t.Error("DevCA certificate IsCA = false, want true")
	}
	if caCert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("DevCA certificate missing KeyUsageCertSign")
	}
	dir, _ := devCertDir()
	if _, err := os.Stat(filepath.Join(dir, devCAFilename)); err != nil {
		t.Fatalf("expected ca cert on disk: %v", err)
	}
}

// Stage 2 test — DevCertSPKI returns the leaf's SPKI hash and differs from the CA's.
func TestDevCertSPKI_PointsToLeafNotCA(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubInterfaceAddrs(t, "192.168.1.50/24")

	spki, err := DevCertSPKI()
	if err != nil {
		t.Fatalf("DevCertSPKI: %v", err)
	}

	leafDER, err := devCertLeafDER()
	if err != nil {
		t.Fatalf("devCertLeafDER: %v", err)
	}
	leafCert, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("parsing leaf cert: %v", err)
	}
	leafSum := sha256.Sum256(leafCert.RawSubjectPublicKeyInfo)
	expectedLeafSPKI := base64.StdEncoding.EncodeToString(leafSum[:])

	caDER, err := DevCA()
	if err != nil {
		t.Fatalf("DevCA: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parsing CA cert: %v", err)
	}
	caSum := sha256.Sum256(caCert.RawSubjectPublicKeyInfo)
	caSPKI := base64.StdEncoding.EncodeToString(caSum[:])

	if spki != expectedLeafSPKI {
		t.Errorf("DevCertSPKI() = %q, want leaf SPKI %q", spki, expectedLeafSPKI)
	}
	if spki == caSPKI {
		t.Errorf("DevCertSPKI() unexpectedly equal to CA SPKI %q", caSPKI)
	}
}

// Stage 3 consumer-shaped test — starts server with DevTLS, fetches /__webtyp/ca,
// and asserts all 4 required criteria.
func TestDevTLS_CAEndpointConsumerShaped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubInterfaceAddrs(t, "127.0.0.1/8")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving port: %v", err)
	}
	port := strings.Split(ln.Addr().String(), ":")[1]
	ln.Close()

	s := New(Config{
		Port:   port,
		Health: true,
		TLS: TLSConfig{
			DevTLS: true,
		},
	})

	// Serve CAPath explicitly on s.Router() as strategies.go / maingen.go do
	s.Router().PublicAsset(CAPath, func(c router.Context) {
		der, err := DevCA()
		if err != nil {
			c.WriteStatus(http.StatusServiceUnavailable)
			return
		}
		c.SetHeader("Content-Type", CADownloadContentType)
		c.Write(der)
	})

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.ListenAndServe()
	}()

	select {
	case err := <-errChan:
		t.Fatalf("Server failed to start: %v", err)
	case <-time.After(1 * time.Second):
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	url := fmt.Sprintf("https://127.0.0.1:%s%s", port, CAPath)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Assertion 4: response Content-Type is application/x-x509-ca-cert.
	ct := resp.Header.Get("Content-Type")
	if ct != CADownloadContentType {
		t.Errorf("Content-Type = %q, want %q", ct, CADownloadContentType)
	}

	caBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}

	// Assertion 1: parses as an X.509 certificate.
	caCert, err := x509.ParseCertificate(caBytes)
	if err != nil {
		t.Fatalf("failed to parse CA certificate from /__webtyp/ca: %v", err)
	}

	// Assertion 2: IsCA is true and KeyUsage includes KeyUsageCertSign.
	if !caCert.IsCA {
		t.Error("CA certificate IsCA = false, want true")
	}
	if caCert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("CA certificate missing KeyUsageCertSign")
	}

	// Verify openssl basicConstraints output (Acceptance Criteria 2 & 3)
	if openssl, err := exec.LookPath("openssl"); err == nil {
		tmpDir := t.TempDir()
		caPath := filepath.Join(tmpDir, "ca.crt")
		leafPath := filepath.Join(tmpDir, "leaf.crt")

		_ = os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes}), 0644)
		_ = os.WriteFile(leafPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: resp.TLS.PeerCertificates[0].Raw}), 0644)

		outCA, err1 := exec.Command(openssl, "x509", "-in", caPath, "-noout", "-ext", "basicConstraints").CombinedOutput()
		if err1 != nil || !strings.Contains(string(outCA), "CA:TRUE") {
			t.Errorf("openssl basicConstraints for CA = %q (err %v), want CA:TRUE", string(outCA), err1)
		}

		outLeaf, err2 := exec.Command(openssl, "x509", "-in", leafPath, "-noout", "-ext", "basicConstraints").CombinedOutput()
		if err2 != nil || !strings.Contains(string(outLeaf), "CA:FALSE") {
			t.Errorf("openssl basicConstraints for Leaf = %q (err %v), want CA:FALSE", string(outLeaf), err2)
		}
	}

	// Assertion 3: leaf presented by TLS handshake verifies against CA using x509.CertPool.
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		t.Fatal("no peer certificates presented in TLS handshake")
	}
	leafCert := resp.TLS.PeerCertificates[0]
	if leafCert.IsCA {
		t.Error("presented leaf certificate has IsCA = true, want false")
	}

	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	opts := x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "localhost",
		CurrentTime: time.Now(),
	}
	if _, err := leafCert.Verify(opts); err != nil {
		t.Errorf("leaf certificate failed to verify against CA: %v", err)
	}
}
