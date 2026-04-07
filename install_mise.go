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
	resp, err := http.Get("https://github.com/jdx/mise/releases/latest")
	must(err)
	defer resp.Body.Close()
	u := resp.Request.URL.String()
	return u[strings.LastIndex(u, "/tag/")+5:]
}

func downloadArchive(tag, dir string) string {
	archive := dir + "/mise.tar.gz"
	url := fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/mise-%s-linux-x64.tar.gz", tag, tag)

	resp, err := http.Get(url)
	must(err)
	defer resp.Body.Close()

	out, err := os.Create(archive)
	must(err)
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	must(err)
	return archive
}

func extractMise(archive, dir string) {
	f, err := os.Open(archive)
	must(err)
	defer f.Close()

	gz, err := gzip.NewReader(f)
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

func cleanup(path string) {
	_ = os.Remove(path)
}

func main() {
	tag := resolveTag()
	dir := localBin()
	archive := downloadArchive(tag, dir)
	extractMise(archive, dir)
	cleanup(archive)
	fmt.Println("mise installé dans", dir+"/mise")
}
