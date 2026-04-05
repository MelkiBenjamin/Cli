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

//func must[T any](val T, err error, context string) T {
//	if err != nil {
	//	_, file, line, _ := runtime.Caller(1)
	//	log.Fatalf("❌ %s\n📍 %s:%d\n➡️ %v", context, line, err)
	//}
	//return val
//}

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
}

// extractFile : Logique commune sans dépendance à filepath
func extractFile(name string, mode os.FileMode, isDir bool, r io.Reader, destDir string) {
	if isDir || mode&0111 == 0 {
		return
	}

	// 2. Extraction du nom de fichier (équivalent de filepath.Base)
	filename := name
	if i := strings.LastIndex(filename, "/"); i != -1 {
		filename = filename[i+1:]
	}
	
	// Si après nettoyage le nom est vide, on ignore
	if filename == "" {
		return
	}

	// 3. Construction du chemin de destination
	destPath := strings.TrimSuffix(destDir, "/") + "/" + filename

	outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		log.Fatalf("Erreur création %s : %v", destPath, err)
	}
	defer outFile.Close()

	if _, err = io.Copy(outFile, r); err != nil {
		log.Fatalf("Erreur copie vers %s : %v", destPath, err)
	}

	log.Printf("Fichier extrait : %s", destPath)
}

func extractZip(src, dest string) {
	log.Printf("extrait-zip")
	zipReader, err := zip.OpenReader(src)
	if err != nil {
		log.Fatalf("Erreur zip %s : %v", src, err)
	}
	defer zipReader.Close()

	for _, file := range zipReader.File {
		f, err := file.Open()
		if err != nil {
			log.Fatalf("Erreur open zip : %v", err)
		}
		
		extractFile(file.Name, file.FileInfo().Mode(), file.FileInfo().IsDir(), f, dest)
		f.Close()
	}
}

func extractTarGz(src, dest string) {
	log.Printf("extrait-tar")
	file, err := os.Open(src)
	if err != nil {
		log.Fatalf("Erreur tar open %s : %v", src, err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		log.Fatalf("Erreur gzip %s : %v", src, err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Erreur lecture tar : %v", err)
		}

		isDir := header.Typeflag == tar.TypeDir
		extractFile(header.Name, header.FileInfo().Mode(), isDir, tarReader, dest)
	}
	log.Printf("extrait-tar-fait")
}

// Gère le fichier téléchargé : tar / zip / chmod
func handleFile(dest, url string) {
	log.Printf("extraction")
	bin := localBin()
	
	if strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz") {
		log.Printf("tar")
		extractTarGz(dest, bin)
		os.Remove(dest)
	} else if strings.HasSuffix(url, ".zip") {
		log.Printf("zip")
	    extractZip(dest, bin)
		os.Remove(dest)
	}
	os.Chmod(dest, 0755)
}

// Installe un outil
func install(name, url string) {
    fileName := name
	if i := strings.LastIndex(url, "/"); i != -1 && i != len(url)-1 {
		fileName = url[i+1:]
	}	
	dest := localBin() + "/" + fileName

	downloadFile(url, dest)
	handleFile(dest, url)
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
