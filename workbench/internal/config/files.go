package config

import (
	"os"
	"path/filepath"
)

var (
	CAFile         = configFile("ca.pem")         // CA certificate file
	ServerCertFile = configFile("server.pem")     // Server certificate file
	ServerKeyFile  = configFile("server-key.pem") // Server private key file
)

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
