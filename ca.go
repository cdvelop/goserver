package server

import "webtyp.com/server/httpd"

// CAPath is where the running dev server serves the development certificate so a
// device on the LAN can install it. Re-exported from httpd for callers that only
// import this package (e.g. the TUI that prints a QR code for it).
const CAPath = httpd.CAPath

// CADownloadContentType is the MIME type a CA certificate is served with at
// CAPath.
const CADownloadContentType = httpd.CADownloadContentType
