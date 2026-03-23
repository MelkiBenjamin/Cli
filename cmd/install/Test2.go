package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// Fonction pour récupérer le dossier ~/.local/bin
func localBin() string {
	dir := os.Getenv("HOME") + "/.local/bin"
	os.MkdirAll(dir, 0755) // Crée le dossier si nécessaire
	return dir
}

func downloadFile(url, dest string) {
	log.Printf("⬇ Downloading: %s → %s\n", url, dest)
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

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("telechargement-fait")
}

func extractZip(src, dest string) {
	log.Printf("extrait-zip")
	zipReader, _ := zip.OpenReader(src)
	file := zipReader.File[0] // On prend le premier fichier (unique)
	outFile, _ := os.Create(file.Name) // Utilise le même nom de fichier que dans l'archive
	inFile, _ := file.Open()
	_, _ = outFile.ReadFrom(inFile)
	zipReader.Close(); outFile.Close(); inFile.Close()
	log.Printf("extrait-zip-fait")
}

func extractTarGz(src, dest string) {
	log.Printf("extrait-tar")
	file, _ := os.Open(src)
	gzipReader, _ := gzip.NewReader(file)
	tarReader := tar.NewReader(gzipReader)
	header, _ := tarReader.Next()
	outFile, _ := os.Create(header.Name) // Utilise le même nom de fichier que dans l'archive
	_, _ = outFile.ReadFrom(tarReader)
	file.Close(); gzipReader.Close(); outFile.Close()
	log.Printf("extrait-tar-fait")
}

// Gère le fichier téléchargé : tar / zip / chmod
func handleFile(dest, url, name string) {
	log.Printf("extraction")
	bin := localBin()
	
    var err error
	if strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz") {
		log.Printf("tar")
		extractTarGz(dest, bin)
		os.Remove(dest)
	} else if strings.HasSuffix(url, ".zip") {
		log.Printf("zip")
	    extractZip(dest, bin)
		os.Remove(dest)
	} else {
	    os.Chmod(dest, 0755)
	}
	if err != nil {
		log.Fatalf("Erreur traitement du fichier %s : %v", name, err)
	}
}

// Installe un outil
func install(name, url string) {
	dest := localBin() + "/" + name
	downloadFile(url, dest)
	handleFile(dest, url, name)
}

// Lecture du config.json
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

// Liste des outils
var tools = map[string]string{
	"kubectl":   "https://dl.k8s.io/release/v1.34.1/bin/linux/amd64/kubectl",
	"kompose":   "https://github.com/kubernetes/kompose/releases/latest/download/kompose-linux-amd64",
	"helm":      "https://get.helm.sh/helm-v3.16.1-linux-amd64.tar.gz",
	"helmify":   "https://github.com/arttor/helmify/releases/latest/download/helmify_Linux_x86_64.tar.gz",
	"terraform": "https://releases.hashicorp.com/terraform/1.14.7/terraform_1.14.7_linux_amd64.zip",
	"k3s":       "https://github.com/k3s-io/k3s/releases/download/v1.35.1%2Bk3s1/k3s",
	"docker":    "https://download.docker.com/linux/static/stable/x86_64/docker-29.3.0.tgz",
	"dockerizer": "https://github.com/MelkiBenjamin/Cli/raw/refs/heads/main/my-artifact.zip",
}

func main() {
	config := readConfig()

	for name := range config {
		url, ok := tools[name]
		if !ok {
			continue
		}
		install(name, url)

		if name == "kubectl" {
			install("kompose", tools["kompose"])
        } else if name == "helm" {
			install("helmify", tools["helmify"])
		} else if name == "docker" {
			install("dockerizer", tools["dockerizer"])
		}
	}
	log.Println("Installation terminée")
}

