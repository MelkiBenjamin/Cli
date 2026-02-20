package main

// Minimal proof-of-concept: read a minimal YAML config, download a file,
// verify sha256 from a sums file, install to /usr/local/bin.
// Usage: go build -o install-k3s && sudo ./install-k3s -config install-config.yaml -name k3s -install /usr/local/bin/k3s

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type Component map[string]string

type Config struct {
	Components []map[string]map[string]string `yaml:"components"`
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg Config
	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func findComponent(cfg *Config, name string) map[string]string {
	for _, c := range cfg.Components {
		if props, ok := c[name]; ok {
			return props
		}
	}
	return nil
}

func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func sha256OfFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func verifyFromSums(filePath, sumsPath, needle string) (bool, error) {
	f, err := os.Open(sumsPath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	var expected string
	// simple parse: look for line with trailing filename
	buf := make([]byte, 4*1024)
	data := make([]byte, 0)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
	}
	// naive: split lines
	for _, line := range splitLines(string(data)) {
		// look for "sha256  filename" or "sha filename"
		if len(line) == 0 {
			continue
		}
		// simplistic split
		var hash, name string
		fmt.Sscanf(line, "%s %s", &hash, &name)
		if name == needle || filepath.Base(name) == needle {
			expected = hash
			break
		}
	}
	if expected == "" {
		return false, errors.New("needle not found in sums file")
	}
	actual, err := sha256OfFile(filePath)
	if err != nil {
		return false, err
	}
	return actual == expected, nil
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i, ch := range s {
		if ch == '\n' || ch == '\r' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func installBinary(tmpPath, target string) error {
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return err
	}
	return os.Rename(tmpPath, target)
}

func main() {
	cfgPath := flag.String("config", "install-config.yaml", "config file")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load config:", err)
		os.Exit(2)
	}

	// Example: install k3s if present
	if c := findComponent(cfg, "k3s"); c != nil {
		ver := c["version"]
		if ver == "" {
			fmt.Fprintln(os.Stderr, "k3s.version missing")
			os.Exit(2)
		}
		tag := ver // assume tag is usable as in script
		base := "https://github.com/k3s-io/k3s/releases/download/" + tag
		bin := "/tmp/k3s-bin"
		sums := "/tmp/sha256sums.txt"

		if err := downloadFile(base+"/k3s", bin); err != nil {
			fmt.Fprintln(os.Stderr, "download k3s:", err)
			os.Exit(2)
		}
		if err := downloadFile(base+"/sha256sum-amd64.txt", sums); err != nil {
			_ = os.Remove(bin)
			fmt.Fprintln(os.Stderr, "download sums:", err)
			os.Exit(2)
		}
		ok, err := verifyFromSums(bin, sums, "k3s")
		if err != nil || !ok {
			_ = os.Remove(bin)
			fmt.Fprintln(os.Stderr, "checksum failed:", err)
			os.Exit(2)
		}
		if err := installBinary(bin, "/usr/local/bin/k3s"); err != nil {
			fmt.Fprintln(os.Stderr, "install:", err)
			os.Exit(2)
		}
		fmt.Println("k3s installed")
	}
}