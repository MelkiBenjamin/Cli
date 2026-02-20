package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Installer simple : télécharge un binaire + fichier de sums, vérifie sha256 et installe.
// Prise en charge d'un fichier YAML (par défaut install-config.yaml) avec la forme:
// k3s:
//   version: v1.35.1+k3s1
//   install: /usr/local/bin/k3s
//
// Flags (priorité) : -bin -name -install -config

func fatalf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func downloadTo(pathOnDisk, urlStr string) error {
	resp, err := http.Get(urlStr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %s returned %s", urlStr, resp.Status)
	}
	out, err := os.Create(pathOnDisk)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func sha256Of(path string) (string, error) {
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

func findHashInSums(sumsPath, needle string) (string, error) {
	f, err := os.Open(sumsPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var hash, name string
		_, _ = fmt.Sscanf(scanner.Text(), "%s %s", &hash, &name)
		if name == needle || filepath.Base(name) == needle {
			return hash, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("needle %q not found in sums", needle)
}

func installBinary(tmpPath, target string) error {
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return err
	}
	return os.Rename(tmpPath, target)
}

/* YAML parsing */
type yamlConfig struct {
	K3s        map[string]string              `yaml:"k3s"`
	Components []map[string]map[string]string `yaml:"components"`
}

func loadConfig(cfgPath string) (version string, installPath string, binURL string, err error) {
	f, err := os.Open(cfgPath)
	if err != nil {
		return "", "", "", err
	}
	defer f.Close()
	var cfg yamlConfig
	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		return "", "", "", err
	}
	if cfg.K3s != nil {
		if v, ok := cfg.K3s["version"]; ok && v != "" {
			version = v
		}
		if p, ok := cfg.K3s["install"]; ok && p != "" {
			installPath = p
		}
		if b, ok := cfg.K3s["bin"]; ok && b != "" {
			binURL = b
		}
	}
	// fallback to components list
	if version == "" && len(cfg.Components) > 0 {
		for _, comp := range cfg.Components {
			if props, ok := comp["k3s"]; ok {
				if v, ok2 := props["version"]; ok2 && v != "" {
					version = v
					if p, ok3 := props["install"]; ok3 {
						installPath = p
					}
					break
				}
			}
		}
	}
	return version, installPath, binURL, nil
}

func main() {
	fmt.Println("debut du script")

	binURL := flag.String("bin", "", "binary URL to download")
	name := flag.String("name", "", "needle filename in sums, e.g. k3s")
	installPath := flag.String("install", "", "install destination, e.g. /usr/local/bin/k3s")
	configPath := flag.String("config", "install-config.yaml", "YAML config file (optional)")
	flag.Parse()

	// Load YAML config if exists (flags have priority)
	cfgVersion, cfgInstall, cfgBin := "", "", ""
	if _, err := os.Stat(*configPath); err == nil {
		if v, p, b, err := loadConfig(*configPath); err == nil {
			cfgVersion, cfgInstall, cfgBin = v, p, b
		} else {
			fmt.Fprintln(os.Stderr, "warning: failed to read config:", err)
		}
	}

	// Use YAML-provided bin if flags not set
	if *binURL == "" && cfgBin != "" {
		*binURL = cfgBin
	}
	// If bin not provided but version in YAML, build common GitHub release asset URL
	if *binURL == "" && cfgVersion != "" {
		escaped := url.PathEscape(cfgVersion)
		*binURL = fmt.Sprintf("https://github.com/k3s-io/k3s/releases/download/%%s/k3s", escaped)
	}
	// Name default to k3s if not set
	if *name == "" && (cfgVersion != "" || *binURL != "") {
		*name = "k3s"
	}
	// install path from YAML if not given as flag
	if *installPath == "" && cfgInstall != "" {
		*installPath = cfgInstall
	}

	// final fallbacks (test defaults)
	defaults := false
	if *binURL == "" {
		*binURL = "https://github.com/k3s-io/k3s/releases/download/v1.35.1%2Bk3s1/k3s"
		defaults = true
	}
	if *name == "" {
		*name = "k3s"
		defaults = true
	}
	if *installPath == "" {
		*installPath = "/tmp/k3s"
		defaults = true
	}
	if defaults {
		fmt.Println("Using defaults/test values:")
		fmt.Println("  - bin     =", *binURL)
		fmt.Println("  - name    =", *name)
		fmt.Println("  - install =", *installPath)
	}

	tmpBin := filepath.Join(os.TempDir(), "tmp-"+*name)
	tmpSums := filepath.Join(os.TempDir(), "tmp-sums.txt")

	if err := downloadTo(tmpBin, *binURL); err != nil {
		fatalf("download binary: %%v", err)
	}

	// build sums URL from binary URL
	u, err := url.Parse(*binURL)
	if err != nil {
		os.Remove(tmpBin)
		fatalf("parse bin url: %%v", err)
	}
	sumsPath := path.Join(path.Dir(u.Path), "sha256sum-amd64.txt")
	sumsURL := fmt.Sprintf("%%s://%%s%%s", u.Scheme, u.Host, sumsPath)

	if err := downloadTo(tmpSums, sumsURL); err != nil {
		os.Remove(tmpBin)
		fatalf("download sums: %%v (tried %%s)", err, sumsURL)
	}

	expected, err := findHashInSums(tmpSums, *name)
	if err != nil {
		os.Remove(tmpBin)
		os.Remove(tmpSums)
		fatalf("find hash: %%v", err)
	}

	actual, err := sha256Of(tmpBin)
	if err != nil {
		os.Remove(tmpBin)
		os.Remove(tmpSums)
		fatalf("sha256: %%v", err)
	}

	if actual != expected {
		os.Remove(tmpBin)
		os.Remove(tmpSums)
		fatalf("checksum mismatch: expected %%s got %%s", expected, actual)
	}

	if err := installBinary(tmpBin, *installPath); err != nil {
		os.Remove(tmpBin)
		os.Remove(tmpSums)
		fatalf("install: %%v", err)
	}

	os.Remove(tmpSums)
	fmt.Println("install ok:", *installPath)
}