package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const latestURL = "https://github.com/jdx/mise/releases/download/v2026.4.6/mise-v2026.4.6-linux-x64.tar.gz"

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func localBin() string {
	home, err := os.UserHomeDir()
	must(err)
	dir := filepath.Join(home, ".local", "bin")
	must(os.MkdirAll(dir, 0o755))
	return dir
}

func extractMiseFromURL(url, dir string) string {
	resp, err := http.Get(url)
	must(err)
	defer resp.Body.Close()

	buffered := bufio.NewReaderSize(resp.Body, 128*1024)
	gz, err := gzip.NewReader(buffered)
	must(err)
	defer gz.Close()

	misePath := filepath.Join(dir, "mise")
	tr := tar.NewReader(gz)

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		must(err)

		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(h.Name, "/mise") {
			continue
		}

		bin, err := os.OpenFile(misePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		must(err)

		_, err = io.Copy(bin, tr)
		must(err)
		must(bin.Close())

		return misePath
	}
	panic("binaire mise introuvable")
}

func readTools(jsonFile string) []string {
	file, err := os.Open(jsonFile)
	must(err)
	defer file.Close()

	var tools []string
	must(json.NewDecoder(file).Decode(&tools))

	return tools
}

type Tool struct {
	Name    string
	Version string
	URL     string
}

var bundles = map[string][]Tool{
	"helm": {
		{Name: "aqua:helm/helm", Version: "3.14.0"},
		{Name: "aqua:arttor/helmify", Version: "0.4.0"},
	},
	"kubectl": {
		{Name: "aqua:kubernetes/kubectl", Version: "1.29.0"},
		{Name: "aqua:kubernetes/kompose", Version: "1.31.0"},
	},
	"terraform": {
		{Name: "terraform", Version: "1.8.5"},
	},
	"k3s": {
		{Name: "k3s", Version: "1.29.3+k3s1"},
	},
	"docker": {
		{
			Name:    "docker",
			Version: "29.3.0",
			URL:     "https://download.docker.com/linux/static/stable/x86_64/docker-29.3.0.tgz",
		},
		{
			Name:    "dockerizer.dev",
			Version: "1.0.0",
			URL:     "https://github.com/MelkiBenjamin/Cli/raw/refs/heads/main/my-artifact.zip",
		},
	},
}
	
func expand(tools []string) []Tool {
	var result []Tool

	for _, t := range tools {
		if bundle, ok := bundles[t]; ok {
			result = append(result, bundle...)
		}
	}
	return result
}

func runMiseUse(misePath string, tools []Tool) {
	for _, t := range tools {

		args := []string{
			"use",
			t.Name + "@" + t.Version,
		}

		if t.URL != "" {
			args = append(args, "--url", t.URL)
		}	
	    args := append([]string{"use"}, tools...)
	    cmd := exec.Command(misePath, args...)
	    cmd.Stdout = os.Stdout
	    cmd.Stderr = os.Stderr

	    must(cmd.Run())
}

func main() {
	dir := localBin()
	misePath := extractMiseFromURL(latestURL, dir)
	fmt.Println("mise installé dans", misePath)

	tools := readTools("Install.json")
	expanded := expandTools(tools)
}
