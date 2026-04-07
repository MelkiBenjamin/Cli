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

func localBon() string {
	dir := os.Getenv("HOME") + "/.local/bon"
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	tag := "latest"
	if len(os.Args) > 1 {
		tag = os.Args[1]
	}
	if tag == "latest" {
		resp, _ := http.Get("https://github.com/jdx/mise/releases/latest")
		_ = resp.Body.Close()
		u := resp.Request.URL.String()
		tag = u[strings.LastIndex(u, "/tag/")+5:]
	}

	dir := localBon()
	archive := dir + "/mise.tar.gz"
	url := fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/mise-%s-linux-x64.tar.gz", tag, tag)

	resp, _ := http.Get(url)
	defer resp.Body.Close()
	out, _ := os.Create(archive)
	_, _ = io.Copy(out, resp.Body)
	_ = out.Close()

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
		_, _ = io.Copy(bin, tr)
		_ = bin.Close()
		break
	}

	_ = os.Remove(archive)
	fmt.Println("mise installé dans", dir+"/mise")
}
