package httpd

import "errors"

// CAPath is where the development certificate authority is served so a device on
// the LAN can install it and trust the dev server. The internal dev server and
// the generated server main both serve DevCA() here with CADownloadContentType.
//
// iOS needs two steps and hints at neither: install the profile (Settings →
// Profile Downloaded), then enable it under Settings → General → About →
// Certificate Trust Settings. A profile installed but not trusted behaves
// exactly like no profile at all.
const CAPath = "/__webtyp/ca"

// CADownloadContentType is the MIME type iOS and Android expect for a CA
// certificate offered for installation.
const CADownloadContentType = "application/x-x509-ca-cert"

var errDevCertDecode = errors.New("httpd: development certificate is not valid PEM")
