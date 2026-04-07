package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

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

func resolveTag() string {
	if len(os.Args) > 1 && os.Args[1] != "" {
		return os.Args[1]
	}
	resp, err := http.Head("https://github.com/jdx/mise/releases/latest")
	must(err)
	defer resp.Body.Close()
	u := resp.Request.URL.String()
	return u[strings.LastIndex(u, "/tag/")+5:]
}

func downloadURL(tag string) string {
	return fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/mise-%s-linux-x64.tar.gz", tag, tag)
}

func extractMiseFromURL(url, dir string) {
	resp, err := http.Get(url)
	must(err)
	defer resp.Body.Close()

	gz, err := gzip.NewReader(resp.Body)
	must(err)
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		must(err)
		parts := strings.Split(h.Name, "/")
		if h.Typeflag != tar.TypeReg || parts[len(parts)-1] != "mise" {
			continue
		}
		bin, err := os.OpenFile(dir+"/mise", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		must(err)
		_, err = io.Copy(bin, tr)
		must(err)
		_ = bin.Close()
		return
	}
	panic("binaire mise introuvable")
}

func main() {
	tag := resolveTag()
	dir := localBin()
	extractMiseFromURL(downloadURL(tag), dir)
	fmt.Println("mise installé dans", dir+"/mise")
}
