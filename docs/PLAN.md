---
PLAN: "fix: the certificate served at /__webtyp/ca cannot be trusted as a CA"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 7725115619946643162
PR: https://github.com/webtyp/server/pull/18
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

## Prerequisite — install the test runner

External agents run in isolated environments where `gotest` is not installed.
Run this **before anything else**; the acceptance criteria depend on it:

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

Then use `gotest` for the whole suite and `gotest -run TestName` for one test.
Never call `go test` directly.

# Plan — serve a real certificate authority

## The defect

The development server serves its certificate at `/__webtyp/ca` so a phone on
the LAN can install it and reach the dev server over HTTPS — the flow that makes
Service Workers and PWA testing possible on a real device.

Inspected against the running server:

```
$ curl -sk -o /dev/null -w '%{http_code} %{content_type}\n' https://localhost:8080/__webtyp/ca
200 application/x-x509-ca-cert

$ openssl s_client -connect localhost:8080 | openssl x509 -noout -issuer -subject -ext basicConstraints,keyUsage
issuer=O=WebTyp Dev CA
subject=O=WebTyp Dev CA
X509v3 Key Usage: critical
    Digital Signature, Key Encipherment
X509v3 Basic Constraints: critical
    CA:FALSE
```

The certificate is a **self-signed leaf**: `issuer == subject`, `CA:FALSE`, and
no `Certificate Sign` key usage. It is served with
`Content-Type: application/x-x509-ca-cert`, which tells the device it is a
certificate authority.

It is not one. A certificate with `CA:FALSE` cannot act as a trust anchor: iOS
and Android will either refuse the profile or install something that never
validates anything. The failure gives no message on the device — the page simply
keeps reporting an invalid certificate after the user has done everything the
instructions asked.

**This does not block `webtyp dev` on the developer's own machine.** Chrome is
handled by an SPKI pin (`webtyp.com/devbrowser`), which pins a public key and
does not care about `CA:FALSE`. Only the phone flow is broken.

## Design gate

**1. Prior art.** `mkcert`, Caddy's internal PKI and Vite's basic-ssl plugin all
generate a **two-level chain**: a long-lived local CA (`CA:TRUE`, `keyCertSign`)
that signs a short-lived leaf carrying the host names. The CA is what a device
installs; the leaf is what the server presents. No mainstream tool asks a device
to trust a leaf, because the platforms do not allow it.

**2. Novice-name test.** `DevCA()` returns the authority; `DevCert()` returns
the certificate the server presents. The distinction is the whole fix, so the
names must carry it. Rejected: keeping one `DevCert` for both — that ambiguity
is what produced the defect.

**3. Ledger.**

```
Concepts to learn                  +1   (a CA distinct from the leaf)
Certificates on disk               +1
Devices that can trust the server  +∞   (from 0 — currently none can)
Silent failure modes               −1
```

**4. Where it belongs.** `server/httpd` already owns certificate generation.

**5. What it deletes.** The self-signed-leaf generation path — replaced, not
kept alongside.

## Stage 1 — generate a CA and a leaf

In `httpd`, replace single-certificate generation with a chain:

- **CA**: `CA:TRUE` with `MaxPathLen: 0`, key usage `keyCertSign | cRLSign`,
  subject `O=WebTyp Dev CA`, validity ~10 years. Persisted; regenerated only
  when missing or expired. It carries **no** subject alternative names — a CA
  is not a server.
- **Leaf**: signed by the CA, key usage `Digital Signature, Key Encipherment`,
  extended key usage `serverAuth`, `CA:FALSE`, and the SAN set already
  implemented — `DNS:localhost`, `IP:127.0.0.1`, `IP:::1` and every non-loopback
  IPv4 of the host's interfaces. Validity 398 days or less, and **regenerated
  whenever the address set changes**, which the current code already does.

The server presents leaf + CA in its TLS chain so a device that trusts the CA
validates without needing the leaf.

```go
// DevCA returns the DER of the development certificate authority — the file a
// device installs to trust this server. It is NOT the certificate the server
// presents; that is the leaf DevCA signed.
func DevCA() ([]byte, error)
```

`/__webtyp/ca` serves `DevCA()`, not the leaf.

## Stage 2 — keep the SPKI pin pointing at the leaf

`DevCertSPKI()` must keep returning the **leaf's** SubjectPublicKeyInfo hash,
not the CA's. Chrome's `--ignore-certificate-errors-spki-list` matches any
certificate in the presented chain, so either would appear to work — but pinning
the CA would make the browser accept every certificate that CA ever signs, which
is a wider grant than intended and would survive a leaf rotation unnoticed.

Add a test asserting the returned hash equals the leaf's and differs from the
CA's. Two hashes that must not be confused need a test saying which is which.

## Stage 3 — the consumer-shaped test

The previous round shipped `DevCertSPKI()` with only an isolated unit test, and
no consumer ever called it; the result was `ERR_CERT_AUTHORITY_INVALID` for
every user of `webtyp dev`. The rule exists for this: *an API is not published
until a consumer-shaped test, inside the library itself, proves it.*

Add a test that starts the server with `DevTLS`, fetches `/__webtyp/ca`, parses
what comes back, and asserts:

1. it parses as an X.509 certificate;
2. `IsCA` is true and `KeyUsage` includes `KeyUsageCertSign`;
3. the leaf presented by the TLS handshake **verifies against it** using
   `x509.CertPool` — the actual thing a phone does;
4. the response `Content-Type` is `application/x-x509-ca-cert`.

Assertion 3 is the regression test for this defect. It fails today.

## Constraints

- **No hardcoded strings.** Subject, path, content type and messages are named
  constants.
- Do not keep the old single-certificate path behind a flag. It is replaced.
- Backend tooling: the standard library is legitimate here.

## Acceptance criteria

1. `gotest` passes, including the four assertions above.
2. `openssl x509 -noout -ext basicConstraints` on the bytes from `/__webtyp/ca`
   reports `CA:TRUE`.
3. The same command on the certificate presented by the handshake reports
   `CA:FALSE` — the leaf must not become a CA.
4. `DevCertSPKI()` equals the leaf's SPKI hash, asserted in a test.

## Stages

| # | Stage | File(s) | Gate |
|---|---|---|---|
| 1 | CA + leaf chain, `DevCA()` | `httpd/tls.go`, `httpd/devcert.go` | criteria 2, 3 |
| 2 | SPKI stays on the leaf | `httpd/spki.go` | criterion 4 |
| 3 | consumer-shaped test | `httpd/devcert_internal_test.go` | criterion 1 |

Sequential.

## Out of scope

Trusting the CA on the developer's own machine. Chrome is handled by the SPKI
pin in `webtyp.com/devbrowser`; installing a CA into the OS trust store was
rejected earlier and stays rejected.
