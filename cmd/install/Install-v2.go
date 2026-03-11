package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
)

func localBin() string {
	dir := os.Getenv("HOME") + "/.local/bin"
	os.MkdirAll(dir, 0755)
	return dir
}

func run(cmd string, args ...string) {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		log.Fatal(err)
	}
}

func download(url, dest string) {
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	out, err := os.Create(dest)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	io.Copy(out, resp.Body)
}

func installDocker() {
	url := "https://download.docker.com/linux/static/stable/x86_64/docker-24.0.5.tgz"
	dest := localBin() + "/docker.tgz"

	download(url, dest)

	run("tar", "-xzf", dest, "-C", "localBin()")
	run("rm", dest)
}

func installTerraform() {
	url := "https://releases.hashicorp.com/terraform/1.6.4/terraform_1.6.4_linux_amd64.zip"
	dest := localBin() + "/terraform.zip"

	download(url, dest)

	run("unzip", "-o", dest, "-d", "localBin()")
	run("rm", dest)
}

func installK3s() {
	url := "https://github.com/k3s-io/k3s/releases/download/v1.27.5+k3s1/k3s"
	dest := localBin() + "/k3s"

	download(url, dest)
	run("chmod", "+x", dest)
}

func main() {
	installDocker()
	installTerraform()
	installK3s()
	log.Println("installation terminée")
}
