package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"encoding/json"
	"os/exec"
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
	dir := home + "/.local/bin"
	must(os.MkdirAll(dir, 0o755))
	return dir
}

func extractMiseFromURL(url, dir string) {
	resp, err := http.Get(url)
	must(err)
	defer resp.Body.Close()

	buffered := bufio.NewReaderSize(resp.Body, 128*1024)
	gz, err := gzip.NewReader(buffered)
	must(err)
	defer gz.Close()

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
		bin, err := os.OpenFile(dir+"/mise", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		must(err)
		_, err = io.Copy(bin, tr)
		must(err)
		_ = bin.Close()
		return
	}
//	panic("binaire mise introuvable")
}

func installTools(misePath string, jsonFile string) {
	file, err := os.Open(jsonFile)
	must(err)
	defer file.Close()

	var tools []string
	err = json.NewDecoder(file).Decode(&tools)
	must(err)

	// Tu peux ajuster la commande selon tes besoins (ex: "mise install")
	args := append([]string{"run"}, tools...)
	cmd := exec.Command(misePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

    err = cmd.Run()
	must(err)
}

func main() {
	dir := localBin()
	extractMiseFromURL(latestURL, dir)
	fmt.Println("mise installé dans", dir+"/mise")
	installTools("mise", "Install.json")
}
