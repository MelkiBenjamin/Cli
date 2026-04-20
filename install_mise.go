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

type Mode string

const (
	ModeUse     Mode = "use"
	ModeInstall Mode = "install"
)

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

var bundles = map[string][]string{
	"helm": {
		"aqua:helm/helm",
		"aqua:arttor/helmify",
	},
	"kubectl": {
		"aqua:kubernetes/kubectl",
		"aqua:kubernetes/kompose",
	},
	"terraform": {
		"terraform",
	},
	"k3s": {
		"k3s",
	},
    "docker": {
		"http:docker",
		"http:dockerizer"
	}
}

func expandTools(tools []string) []string {
	seen := make(map[string]bool)
	expanded := make([]string, 0)

	for _, t := range tools {
		bundle, ok := bundles[t]
		if !ok {
			continue
		}

		for _, tool := range bundle {
			if !seen[tool] {
				seen[tool] = true
				expanded = append(expanded, tool)
			}
		}
	}
	return expanded
}

func runMiseUse(misePath string, tools []string) {
	args := append([]string{"use"}, tools...)
	cmd := exec.Command(misePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	must(cmd.Run())
}

func runMiseInstall(misePath string, tools []string) {
	args := append([]string{"install"}, tools...)
	cmd := exec.Command(misePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	must(cmd.Run())
}

func main() {
	mode := ModeUse
	if len(os.Args) > 1 {
		mode = Mode(strings.ToLower(os.Args[1]))
	}

	if mode != ModeUse && mode != ModeInstall {
		os.Exit(1)
	}

	dir := localBin()
	misePath := extractMiseFromURL(latestURL, dir)
	fmt.Println("mise installé dans", misePath)

	tools := readTools("Install.json")
	expanded := expandTools(tools)

	switch mode {
	case ModeUse:
		runMiseUse(misePath, expanded)
	case ModeInstall:
		runMiseInstall(misePath, expanded)
	}
}
