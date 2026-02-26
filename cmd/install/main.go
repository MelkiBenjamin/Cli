package main

import (
	"bufio"
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
	configFile  = "install-config.yaml"
	installPath = "/usr/local/bin/k3s"
	defaultURL  = "https://github.com/k3s-io/k3s/releases/download/v1.35.1%2Bk3s1/k3s"
	sumsLocal   = "/usr/local/bin/.k3s.sums"
	sumsName    = "sha256sum-amd64.txt"
)

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}

func readConfig() string {
	f, err := os.Open(configFile)
	if err != nil {
		return ""
	}
	defer f.Close()

	var doc map[string]map[string]string
	_ = yaml.NewDecoder(f).Decode(&doc)
	if k, ok := doc["k3s"]; ok {
		if b := k["bin"]; b != "" {
			return b
		}
		if v := k["version"]; v != "" {
			esc := strings.ReplaceAll(v, "+", "%2B")
			return "https://github.com/k3s-io/k3s/releases/download/" + esc + "/k3s"
		}
	}
	return ""
}

func download(dst, src string) {
	resp, err := http.Get(src)
	if err != nil {
		fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		fatal(fmt.Errorf("http status %s", resp.Status))
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		fatal(err)
	}
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		fatal(err)
	}
	_ = out.Sync()
	_ = out.Close()
}

func buildSumsURL(binURL string) string {
	i := strings.LastIndex(binURL, "/")
	if i < 0 {
		return binURL + "/" + sumsName
	}
	return binURL[:i+1] + sumsName
}

func findHash(sumsPath, needle string) string {
	f, err := os.Open(sumsPath)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := parts[0]
		name := strings.TrimPrefix(parts[1], "*")
		if name == needle || strings.HasSuffix(name, "/"+needle) {
			return hash
		}
	}
	if scanner.Err() != nil {
		fatal(scanner.Err())
	}
	fatal(fmt.Errorf("hash for %s not found in %s", needle, sumsPath))
	return ""
}

func sha256Of(p string) string {
	f, err := os.Open(p)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func main() {
	binURL := readConfig()
	if binURL == "" {
		binURL = defaultURL
	}

	// download binary directly to final path
	download(installPath, binURL)

	// download sums naively into fixed local file (overwrite)
	sumsURL := buildSumsURL(binURL)
	download(sumsLocal, sumsURL)

	expected := findHash(sumsLocal, "k3s")
	actual := sha256Of(installPath)

	if expected != actual {
		fatal(fmt.Errorf("checksum mismatch"))
	}

	_ = os.Chmod(installPath, 0755)
	fmt.Println("install ok:", installPath)
}
