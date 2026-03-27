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

func fileNameFromURL(url, fallback string) string {
	i := strings.LastIndex(url, "/")
	if i == -1 || i == len(url)-1 {
		return fallback
	}
	return url[i+1:]
}

func baseName(p string) string {
	i := strings.LastIndex(p, "/")
	if i == -1 {
		return p
	}
	return p[i+1:]
}

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
	//outPath := dest + "/" + baseName(file.Name)
    //outFile, err := os.Create(outPath)

	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	// 👉 ICI
    log.Println("Status:", resp.Status)
    log.Println("Content-Type:", resp.Header.Get("Content-Type"))
	log.Printf("telechargement-fait")
}

//func extractZip(src, dest string) {
//	log.Printf("extrait-zip")
//	zipReader, err := zip.OpenReader(src)
//	if err != nil {
//		log.Fatalf("Erreur zip traitement du fichier %s : %v", src, err)
//	}
//	file := zipReader.File[0] // On prend le premier fichier (unique)
//	outFile, err := os.Create(file.Name) // Utilise le même nom de fichier que dans l'archive
//	if err != nil {
//		log.Fatalf("Erreur zip de create %s : %v", src, err)
//	}
//	inFile, err := file.Open()
//	if err != nil {
//		log.Fatalf("Erreur zip de file-open %s : %v", src, err)
//	}
//	_, err = outFile.ReadFrom(inFile)
//	if err != nil {
//		log.Fatalf("Erreur zip final %s : %v", src, err)
//	}
//	zipReader.Close(); outFile.Close(); inFile.Close()
//	log.Printf("extrait-zip-fait")
//}

func extractZip(src, dest string) {
	log.Printf("extrait-zip")

	zipReader, err := zip.OpenReader(src)
	if err != nil {
		log.Fatalf("Erreur zip %s : %v", src, err)
	}
	defer zipReader.Close()

	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		if file.FileInfo().Mode()&0111 == 0 {
	        continue
        }

		filename := file.Name
		if i := strings.LastIndex(filename, "/"); i != -1 {
			filename = filename[i+1:]
		}

		destpath := dest + "/" + filename

		inFile, err := file.Open()
		if err != nil {
			log.Fatalf("Erreur open zip %s : %v", src, err)
		}

		outFile, err := os.Create(destpath)
		if err != nil {
			inFile.Close()
			log.Fatalf("Erreur create %s : %v", destpath, err)
		}

		_, err = io.Copy(outFile, inFile)
		outFile.Close()
		inFile.Close()
		if err != nil {
			log.Fatalf("Erreur copy %s : %v", destpath, err)
		}

		if err := os.Chmod(destpath, 0755); err != nil {
			log.Fatalf("Erreur chmod %s : %v", destpath, err)
		}

		log.Printf("Fichier extrait : %s", destpath)
		return
	}

	log.Fatalf("Aucun binaire trouvé dans %s", src)
}


func extractTarGz(src, dest string) {
	log.Printf("extrait-tar")
	//file := must(os.Open(src), "Erreur tar traitement du fichier")
	file, err := os.Open(src)
    if err != nil {
		log.Fatalf("Erreur tar traitement du fichier  %s : %v", src, err)
    }
	
	buf := make([]byte, 2)
    file.Read(buf)
    file.Seek(0, 0)

    log.Printf("Magic bytes: %x\n", buf)
	defer file.Close() // ← on ferme uniquement à la fin
	
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		log.Fatalf("Erreur tar de la creation gzip %s : %v", src, err)
	}
	defer gzipReader.Close() // ← idem
	tarReader := tar.NewReader(gzipReader)
    //	var header *tar.Header
	log.Printf("log de test")
	for {
		log.Printf("debut for")
    	header, err := tarReader.Next()
		if err == io.EOF {
	      log.Println("Fin de l'archive.")
		  break     
		}
		log.Printf("suite 1 for")
		if err != nil {
		  log.Fatalf("Erreur tar lors de la lecture du 1er fichier  %s : %v", src, err)
		}
		log.Printf("suite 2 for")
		// Assurez-vous que l'en-tête n'est pas nil
		if header == nil {
		  log.Println("Ignorer une entrée qui est probablement un dossier ou un lien symbolique.")
		  continue
		}
		log.Printf("Lecture de l'entrée : %s", header.Name)
		log.Printf("suite 3 for")
	    if header.Typeflag == tar.TypeDir {
		  log.Printf("Répertoire ignoré : %s", header.Name)
		  continue
	    }
		log.Printf("suite 4 for")
		if header.Typeflag != tar.TypeReg {
			continue
		}

		if header.FileInfo().Mode()&0111 == 0 {
			continue
		}

     	log.Printf("suite avant création destpath")
		//bin := localBin()
        //destpath := bin + "/" + header.Name // Concaténation avec le nom du fichier extrait
	    // dirpath := dest + "/" + path.Dir(header.Name) // Répertoire de l'extrait
		filename := header.Name
        if i := strings.LastIndex(filename, "/"); i != -1 {
	      filename = filename[i+1:]
        }
        destpath := dest + "/" + filename

        // parts := strings.Split(destpath, "/")
        //dir := strings.Join(parts[:len(parts)-1], "/")

		if destpath == src {
        	log.Fatalf("collision source/destination : %s", destpath)
        }

		// Créer les répertoires manquants
    
		log.Printf("suite avant outfile create")
	
    	outFile, err := os.Create(destpath) // Utilise le même nom de fichier que dans l'archive
    	if err != nil {
	    	log.Fatalf("Erreur de create tar soit %s : %v", destpath, err)
    	}
	
    	_, err = io.Copy(outFile, tarReader)
    	if err != nil {
	    	log.Fatalf("Erreur final %s : %v", destpath, err)
	    }
    	outFile.Close()
		if err := os.Chmod(destpath, 0755); err != nil {
			log.Fatalf("Erreur chmod %s : %v", destpath, err)
		}
		log.Printf("Fichier extrait : %s", destpath)
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
	fileName := fileNameFromURL(url, name)
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
