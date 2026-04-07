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

func localBin() string {
	if custom := os.Getenv("MISE_INSTALL_DIR"); custom != "" {
		_ = os.MkdirAll(custom, 0o755)
		return custom
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		dir := home + "/.local/bin"
		if os.MkdirAll(dir, 0o755) == nil {
			return dir
		}
	}

	fallback := "./.local/bin"
	_ = os.MkdirAll(fallback, 0o755)
	return fallback
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
		if resp != nil {
			_ = resp.Body.Close()
			u := resp.Request.URL.String()
			tag = u[strings.LastIndex(u, "/tag/")+5:]
		}
	}

	dir := localBin()
	archive := dir + "/mise.tar.gz"
	url := fmt.Sprintf("https://github.com/jdx/mise/releases/download/%s/mise-%s-linux-x64.tar.gz", tag, tag)

	resp, _ := http.Get(url)
	if resp == nil {
		panic("download impossible")
	}
	defer resp.Body.Close()
	out, err := os.Create(archive)
	must(err)
	_, err = io.Copy(out, resp.Body)
	must(err)
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
		_, err = io.Copy(bin, tr)
		must(err)
		_ = bin.Close()
		break
	}

	_ = os.Remove(archive)
	fmt.Println("mise installé dans", dir+"/mise", "(sans sudo)")
}
