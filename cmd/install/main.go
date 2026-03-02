package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	configFile     = "install-config.yaml"
	installPath    = "/usr/local/bin/k3s"
	defaultVersion = "v1.35.1+k3s1"
)


// readVersion lit la version dans le YAML.
// Si absente → retourne "".
func readVersion() string {
	f, _ := os.Open(configFile)
	var doc map[string]map[string]string
	_ = yaml.NewDecoder(f).Decode(&doc)
	_ = f.Close()

	return doc["k3s"]["version"]
}


// buildURLs construit les URLs du binaire et du checksum.
func buildURLs(version string) (string, string) {
	escaped := strings.ReplaceAll(version, "+", "%2B")

	base := "https://github.com/k3s-io/k3s/releases/download/" + escaped

	binaryURL := base + "/k3s"
	sumsURL := base + "/sha256sum-amd64.txt"

	return binaryURL, sumsURL
}


// downloadToFile télécharge une URL vers un fichier local.
func downloadToFile(dst, url string) {
	resp, _ := http.Get(url)
	out, _ := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	_, _ = io.Copy(out, resp.Body)
	_ = out.Close()
	_ = resp.Body.Close()
}


// downloadToMemory télécharge une URL en mémoire.
func downloadToMemory(url string) []byte {
	resp, _ := http.Get(url)
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return data
}


// sha256Of calcule le hash SHA256 d’un fichier.
func sha256Of(path string) string {
	f, _ := os.Open(path)
	h := sha256.New()
	_, _ = io.Copy(h, f)
	_ = f.Close()
	return hex.EncodeToString(h.Sum(nil))
}


// installK3s orchestre l’installation complète.
func installK3s(version string) {

	if version == "" {
		version = defaultVersion
	}

	binaryURL, sumsURL := buildURLs(version)

	// téléchargement binaire
	downloadToFile(installPath, binaryURL)

	// téléchargement checksum
	data := downloadToMemory(sumsURL)

	expected := string(data[:64])
	actual := sha256Of(installPath)

	if expected == actual {
		_ = os.Chmod(installPath, 0755)
		fmt.Println("install ok:", installPath)
	}
}


func main() {
	version := readVersion()
	installK3s(version)
}
