// Minimal installer: Terraform (HashiCorp), Docker (Docker Inc.), k3s (Rancher).
// Target: Debian/Ubuntu (amd64). Run as root.
//
// Build: go build -o install_min install_min.go
// Usage: sudo ./install_min
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
)

func run(cmd string, args ...string) {
	log.Printf("=> %s %s\n", cmd, strings.Join(args, " "))
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		log.Fatalf("command failed: %s %v : %v", cmd, args, err)
	}
}

func runOutput(cmd string, args ...string) string {
	c := exec.Command(cmd, args...)
	out, err := c.CombinedOutput()
	if err != nil {
		log.Fatalf("command failed: %s %v : %v\noutput:%s", cmd, args, err, string(out))
	}
	return string(out)
}

func mustRoot() {
	if os.Geteuid() != 0 {
		log.Fatal("Ce script doit être lancé en root (sudo).")
	}
}

func detectDebianLike() {
	data, err := ioutil.ReadFile("/etc/os-release")
	if err != nil {
		log.Fatal("Impossible de lire /etc/os-release : ", err)
	}
	s := strings.ToLower(string(data))
	if !strings.Contains(s, "ubuntu") && !strings.Contains(s, "debian") {
		log.Fatal("Système non supporté. Ce script ne supporte que Debian/Ubuntu.")
	}
}

func aptUpdateAndInstall(pkgs ...string) {
	args := append([]string{"install", "-y"}, pkgs...)
	run("apt-get", "update")
	run("apt-get", args...)
}

func addAptKeyFromURL(url, destFile string) {
	run("mkdir", "-p", "/usr/share/keyrings")
	run("bash", "-c", fmt.Sprintf("curl -fsSL %s | gpg --dearmor -o %s", url, destFile))
}

func writeFile(path, content string) {
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		log.Fatalf("Impossible d'écrire %s: %v", path, err)
	}
}

func installDocker() {
	log.Println("=== Installation Docker (dépôt officiel) ===")
	// Prérequis et clef
	aptUpdateAndInstall("ca-certificates", "curl", "gnupg", "lsb-release")
	// Add Docker GPG key
	addAptKeyFromURL("https://download.docker.com/linux/ubuntu/gpg", "/usr/share/keyrings/docker-archive-keyring.gpg")
	// Add repo (signed-by)
	dist := strings.TrimSpace(runOutput("lsb_release", "-cs"))
	repo := fmt.Sprintf("deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu %s stable", dist)
	writeFile("/etc/apt/sources.list.d/docker.list", repo+"\n")
	run("apt-get", "update")
	aptUpdateAndInstall("docker-ce", "docker-ce-cli", "containerd.io")
	log.Println("Docker installé.")
}

func installTerraform() {
	log.Println("=== Installation Terraform (HashiCorp APT) ===")
	// Add HashiCorp GPG key
	addAptKeyFromURL("https://apt.releases.hashicorp.com/gpg", "/usr/share/keyrings/hashicorp-archive-keyring.gpg")
	// Add repo
	dist := strings.TrimSpace(runOutput("lsb_release", "-cs"))
	repo := fmt.Sprintf("deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com %s main", dist)
	writeFile("/etc/apt/sources.list.d/hashicorp.list", repo+"\n")
	run("apt-get", "update")
	aptUpdateAndInstall("terraform")
	log.Println("Terraform installé.")
}

func installK3s() {
	log.Println("=== Installation k3s (installateur officiel) ===")
	tmp := "/tmp/get-k3s.sh"
	// Télécharger le script
	run("curl", "-sfL", "https://get.k3s.io", "-o", tmp)
	// Vérification basique : présence de mots-clés
	b, err := ioutil.ReadFile(tmp)
	if err != nil {
		log.Fatalf("Erreur lecture %s: %v", tmp, err)
	}
	txt := strings.ToLower(string(b))
	if !strings.Contains(txt, "k3s") || !strings.Contains(txt, "rancher") {
		log.Fatalf("Le script k3s téléchargé ne contient pas les mots-clés attendus. Abort.")
	}
	// Rendre exécutable
	run("chmod", "+x", tmp)
	// Exécuter en désactivant composants par défaut pour réduire la surface (traefik, servicelb, local-storage, metrics-server)
	// Note: ajustez ou retirez des flags si vous avez besoin d'un composant.
	cmd := []string{"sh", tmp}
	env := os.Environ()
	env = append(env, `INSTALL_K3S_EXEC=server --disable=traefik --disable=servicelb --disable=local-storage --disable=metrics-server`)
	// Exécuter le script avec environnement personnalisé
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Env = env
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	log.Printf("Exécution de l'installateur k3s (options minimales)...")
	if err := c.Run(); err != nil {
		log.Fatalf("Échec installation k3s: %v", err)
	}
	log.Println("k3s installé.")
}

func main() {
	log.Println("Minimal installer started")
	mustRoot()
	detectDebianLike()

	// Séquence minimale : Docker, Terraform, k3s
	installDocker()
	installTerraform()
	installK3s()

	log.Println("Terminé — Docker, Terraform et k3s installés (minimal).")
	fmt.Println("\nNote: vérifiez /etc/rancher/k3s/k3s.yaml pour kubeconfig et 'docker --version', 'terraform -version'.")
}
