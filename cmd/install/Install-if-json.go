package main

import (
	"encoding/json"
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
	bin := localBin()
	url := "https://download.docker.com/linux/static/stable/x86_64/docker-29.3.0.tgz"
	dest := localBin() + "/docker.tgz"

	download(url, dest)

	run("tar", "-xzf", dest, "-C", bin)
	run("rm", dest)
}

func installTerraform() {
	bin := localBin()
	url := "https://releases.hashicorp.com/terraform/1.14.7/terraform_1.14.7_linux_amd64.zip"
	dest := localBin() + "/terraform.zip"

	download(url, dest)

	run("unzip", "-o", dest, "-d", bin)
	run("rm", dest)
}

func installK3s() {
	url := "https://github.com/k3s-io/k3s/releases/download/v1.35.1%2Bk3s1/k3s"
	dest := localBin() + "/k3s"

	download(url, dest)
	run("chmod", "+x", dest)
}

func readConfig() map[string]bool {
	file, err := os.Open("config.json")
	if err != nil {
		log.Println("config.json absent ou erreur, rien à installer")
		return map[string]bool{}
	}
	defer file.Close()

	var apps []string
	err = json.NewDecoder(file).Decode(&apps)
	if err != nil {
		log.Println("erreur lecture json, rien à installer")
		return map[string]bool{}
	}

	m := make(map[string]bool)
	for _, a := range apps {
		m[a] = true
	}
	return m
}

func main() {
	config := readConfig()

	if config["docker"] {
		installDocker()
	}

	if config["terraform"] {
		installTerraform()
	}

	if config["k3s"] {
		installK3s()
	}

	log.Println("installation terminée")
}
