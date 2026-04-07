package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// Fonction pour récupérer le dossier ~/.local/bin
func localBin() string {
	dir := os.Getenv("HOME") + "/.local/bin"
	os.MkdirAll(dir, 0755) // Crée le dossier si nécessaire
	return dir
}

// Fonction pour exécuter une commande système
func run(cmd string, args ...string) {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		log.Fatal(err)
	}
}

// Fonction pour télécharger un fichier depuis une URL vers un fichier local
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
// Fonction générique pour installer un outil
func install(name string, url string) {
	bin := localBin()
	dest := bin + "/" + name

	// Télécharger l'outil dans le dossier approprié
	download(url, dest)

	// Si c'est une archive tar.gz, on l'extrait
	if strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz") {
		run("tar", "-xzf", dest, "-C", bin)
		run("rm", dest) // Supprimer l'archive après extraction
	} else if strings.HasSuffix(url, ".zip") { // Si c'est un fichier zip
		run("unzip", "-o", dest, "-d", bin)
		run("rm", dest) // Supprimer l'archive après extraction
	} else { // C'est un binaire pur
		run("chmod", "+x", dest) // Rendre le binaire exécutable
	}

	// Cas spécifique pour "dockerizer"
	src := "/home/runner/work/Cli/Cli/dockerizer/build/dockerizer"
	if _, err := os.Stat(src); err == nil { // Si le fichier existe
		run("cp", src, bin+"/dockerizer") // Copier dans ~/.local/bin
	}
}

// Fonction pour lire le fichier config.json et récupérer les outils à installer
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

// Liste des outils avec leurs URLs respectives pour téléchargement
var tools = map[string]string{
	"kubectl": "https://dl.k8s.io/release/v1.34.1/bin/linux/amd64/kubectl",
	"kompose": "https://github.com/kubernetes/kompose/releases/latest/download/kompose-linux-amd64",
	"helm": "https://get.helm.sh/helm-v3.16.1-linux-amd64.tar.gz",
	"helmify": "https://github.com/arttor/helmify/releases/latest/download/helmify_Linux_x86_64.tar.gz",
	"terraform": "https://releases.hashicorp.com/terraform/1.14.7/terraform_1.14.7_linux_amd64.zip",
	"k3s": "https://github.com/k3s-io/k3s/releases/download/v1.35.1%2Bk3s1/k3s",
	"docker": "https://download.docker.com/linux/static/stable/x86_64/docker-29.3.0.tgz",
}

func main() {
	// Lire la configuration depuis le fichier config.json
	config := readConfig()

	// Installer chaque outil demandé dans la config.json
	for name := range config {
		url, ok := tools[name]
		if !ok {
			continue // Si l'outil n'est pas dans la liste des tools, passer à l'outil suivant
		}

		install(name, url)

		// Installation d'outils dépendants (logique spécifique)
		if name == "kubectl" {
			install("kompose", tools["kompose"])
		}

		if name == "helm" {
			install("helmify", tools["helmify"])
		}
	}

	log.Println("installation terminée")
}
