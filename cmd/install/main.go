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
	sumsLocal      = "/usr/local/bin/.k3s.sums"
	sumsName       = "sha256sum-amd64.txt"
	defaultVersion = "v1.35.1+k3s1"
)

func readVersion() string {
	f, _ := os.Open(configFile)
	var doc map[string]map[string]string
	_ = yaml.NewDecoder(f).Decode(&doc)
	_ = f.Close()

	if doc == nil {
		return ""
	}

	k := doc["k3s"]
	if k == nil {
		return ""
	}

	if v, ok := k["version"]; ok && v != "" {
		return v
	}

	return ""
}

func buildURL(version, file string) string {
	escaped := strings.ReplaceAll(version, "+", "%2B")
	return "https://github.com/k3s-io/k3s/releases/download/" + escaped + "/" + file
}

func download(dst, src string) {
	resp, _ := http.Get(src)
	out, _ := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	_, _ = io.Copy(out, resp.Body)
	_ = out.Sync()
	_ = out.Close()
	_ = resp.Body.Close()
}

func sha256Of(path string) string {
	f, _ := os.Open(path)
	h := sha256.New()
	_, _ = io.Copy(h, f)
	_ = f.Close()
	return hex.EncodeToString(h.Sum(nil))
}

func main() {
	version := readVersion()
	if version == "" {
		version = defaultVersion
	}

	binaryURL := buildURL(version, "k3s")
	sumsURL := buildURL(version, sumsName)

	download(installPath, binaryURL)
	download(sumsLocal, sumsURL)

	// lecture directe du fichier sums
	data, _ := os.ReadFile(sumsLocal)

	// SHA256 = 64 caractères hex
	expected := string(data[:64])
	actual := sha256Of(installPath)

	if expected == actual {
		_ = os.Chmod(installPath, 0755)
		fmt.Println("install ok:", installPath)
	}
}
