package main

import (
	"archive/tar"
<<<<<<< codex/create-go-script-for-installation
	"bufio"
=======
>>>>>>> MelkiBenjamin-patch-1
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

<<<<<<< codex/create-go-script-for-installation
const latestURL = "https://github.com/jdx/mise/releases/latest/download/mise-linux-x64.tar.gz"

=======
>>>>>>> MelkiBenjamin-patch-1
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

<<<<<<< codex/create-go-script-for-installation
=======
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

>>>>>>> MelkiBenjamin-patch-1
func extractMiseFromURL(url, dir string) {
	resp, err := http.Get(url)
	must(err)
	defer resp.Body.Close()

<<<<<<< codex/create-go-script-for-installation
	buffered := bufio.NewReaderSize(resp.Body, 256*1024)
	gz, err := gzip.NewReader(buffered)
=======
	gz, err := gzip.NewReader(resp.Body)
>>>>>>> MelkiBenjamin-patch-1
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
<<<<<<< codex/create-go-script-for-installation
	dir := localBin()
	extractMiseFromURL(latestURL, dir)
=======
	tag := resolveTag()
	dir := localBin()
	extractMiseFromURL(downloadURL(tag), dir)
>>>>>>> MelkiBenjamin-patch-1
	fmt.Println("mise installé dans", dir+"/mise")
}
