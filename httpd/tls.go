package httpd

import (
	"errors"
	"net/http"

	"golang.org/x/crypto/acme/autocert"
)

type TLSConfig struct {
	AutoCert bool
	Domain   string
	CertFile string
	KeyFile  string
	DevTLS   bool
}

func (s *Server) validateTLS() error {
	modes := 0
	if s.config.TLS.AutoCert {
		modes++
		if s.config.TLS.Domain == "" {
			return errors.New("TLS AutoCert requires a Domain")
		}
	}
	if s.config.TLS.CertFile != "" || s.config.TLS.KeyFile != "" {
		modes++
		if s.config.TLS.CertFile == "" || s.config.TLS.KeyFile == "" {
			return errors.New("TLS CertFile and KeyFile must both be provided")
		}
	}
	if s.config.TLS.DevTLS {
		modes++
	}

	if modes > 1 {
		return errors.New("multiple TLS modes enabled; choose at most one (AutoCert, Cert/Key, or DevTLS)")
	}
	return nil
}

func (s *Server) listenAndServe(srv *http.Server) error {
	if s.config.TLS.AutoCert {
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(s.config.TLS.Domain),
			Cache:      autocert.DirCache("certs"),
		}
		srv.TLSConfig = m.TLSConfig()
		return srv.ListenAndServeTLS("", "")
	}

	if s.config.TLS.CertFile != "" {
		return srv.ListenAndServeTLS(s.config.TLS.CertFile, s.config.TLS.KeyFile)
	}

	if s.config.TLS.DevTLS {
		certFile, keyFile, err := s.getOrCreateDevCert()
		if err != nil {
			return err
		}
		return srv.ListenAndServeTLS(certFile, keyFile)
	}

	return srv.ListenAndServe()
}
