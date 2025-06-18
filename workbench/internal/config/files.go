// Package config provides configuration utilities for the proglog application,
// including file path management and TLS configuration setup.
package config

import (
	"os"
	"path/filepath"
)

var (
	// CAFile is the path to the Certificate Authority (CA) certificate file.
	// Used for verifying certificates in TLS connections.
	CAFile = configFile("ca.pem")

	// ServerCertFile is the path to the server's certificate file.
	// Contains the public certificate for the server's TLS identity.
	ServerCertFile = configFile("server.pem")

	// ServerKeyFile is the path to the server's private key file.
	// Contains the private key corresponding to the server certificate.
	ServerKeyFile = configFile("server-key.pem")
)

// configFile returns the full path to a configuration file.
// It first checks for a CONFIG_DIR environment variable, and if not found,
// defaults to a .proglog directory in the user's home directory.
// If the home directory cannot be determined, it panics.
func configFile(filename string) string {
	if dir := os.Getenv("CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, filename)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(homeDir, ".proglog", filename)
}
