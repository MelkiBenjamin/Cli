package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Minimal Go installer: download a binary and its sums file, verify sha256 and install.
// Usage: sudo go run ./cmd/install -bin <url> -name k3s -install /usr/local/bin/k3s

func fatalf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func downloadTo(path, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %s returned %s", url, resp.Status)
	}
	out, err := os.Create(path)
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

func main() {
	fmt.Println("debut du script")

	binURL := flag.String("bin", "", "binary URL to download")
	name := flag.String("name", "", "needle filename in sums, e.g. k3s")
	installPath := flag.String("install", "", "install destination, e.g. /usr/local/bin/k3s")
	flag.Parse()

	if *binURL == "" || *name == "" || *installPath == "" {
		fatalf("usage: -bin <url> -name <filename> -install <path>")
	}

	tmpBin := filepath.Join(os.TempDir(), "tmp-"+*name)
	tmpSums := filepath.Join(os.TempDir(), "tmp-sums.txt")

	if err := downloadTo(tmpBin, *binURL); err != nil {
		fatalf("download binary: %v", err)
	}

	sumsURL := filepath.Dir(*binURL) + "/sha256sum-amd64.txt"
	if err := downloadTo(tmpSums, sumsURL); err != nil {
		os.Remove(tmpBin)
		fatalf("download sums: %v (tried %s)", err, sumsURL)
	}

	expected, err := findHashInSums(tmpSums, *name)
	if err != nil {
		os.Remove(tmpBin)
		os.Remove(tmpSums)
		fatalf("find hash: %v", err)
	}

	actual, err := sha256Of(tmpBin)
	if err != nil {
		os.Remove(tmpBin)
		os.Remove(tmpSums)
		fatalf("sha256: %v", err)
	}

	if actual != expected {
		os.Remove(tmpBin)
		os.Remove(tmpSums)
		fatalf("checksum mismatch: expected %s got %s", expected, actual)
	}

	if err := installBinary(tmpBin, *installPath); err != nil {
		os.Remove(tmpBin)
		os.Remove(tmpSums)
		fatalf("install: %v", err)
	}

	os.Remove(tmpSums)
	fmt.Println("install ok:", *installPath)
}