package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// SetupTLSConfig creates and configures a TLS configuration based on the provided TLSConfig.
// It sets up a TLS 1.3 minimum version configuration and handles both server and client scenarios.
func SetupTLSConfig(cfg TLSConfig) (*tls.Config, error) {
	var err error
	// TLS 1.3を最小バージョンとして設定
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}

	// 証明書と秘密鍵が指定されている場合、それらを読み込む
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, err
		}
	}

	// CA証明書が指定されている場合の処理
	if cfg.CAFile != "" {
		// CA証明書ファイルを読み込む
		b, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, err
		}

		// CA証明書プールを作成してCA証明書を追加
		ca := x509.NewCertPool()
		ok := ca.AppendCertsFromPEM([]byte(b))
		if !ok {
			return nil, fmt.Errorf("failed to parse CA certificate %q", cfg.CAFile)
		}

		// サーバーモードかクライアントモードかで設定を分岐
		if cfg.Server {
			// サーバーモード：クライアント証明書の検証を必須にする
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
			tlsConfig.ClientCAs = ca
		} else {
			// クライアントモード：ルートCA証明書を設定
			tlsConfig.RootCAs = ca
		}
		// サーバー名を設定
		tlsConfig.ServerName = cfg.ServerAddress
	}
	return tlsConfig, nil
}

// TLSConfig holds the configuration parameters for setting up TLS connections.
// It supports both server and client configurations with mutual TLS authentication.
type TLSConfig struct {
	// CertFile is the path to the certificate file (PEM format).
	// Required for both server and client when mutual TLS is enabled.
	CertFile string

	// KeyFile is the path to the private key file (PEM format).
	// Must correspond to the certificate in CertFile.
	KeyFile string

	// CAFile is the path to the Certificate Authority file (PEM format).
	// Used to verify peer certificates in mutual TLS scenarios.
	CAFile string

	// ServerAddress is the expected server name for certificate verification.
	// Used in client mode to verify the server's certificate matches this name.
	ServerAddress string

	// Server indicates whether this configuration is for server mode (true) or client mode (false).
	// Affects how CA certificates are used and certificate verification behavior.
	Server bool
}
