package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"net"
	"time"
	"encoding/hex"
	crand "crypto/rand"
)

const latestURL = "https://github.com/jdx/mise/releases/download/v2026.4.24/mise-v2026.4.24-linux-x64-musl.tar.gz"

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func localBin() string {
	home, err := os.UserHomeDir()
	must(err)
	dir := filepath.Join(home, ".local", "bin")
	must(os.MkdirAll(dir, 0o755))
	return dir
}

func extractMiseFromURL(url, dir string) string {
	resp, err := http.Get(url)
	must(err)
	defer resp.Body.Close()

	buffered := bufio.NewReaderSize(resp.Body, 128*1024)
	gz, err := gzip.NewReader(buffered)
	must(err)
	defer gz.Close()

	misePath := filepath.Join(dir, "mise")
	tr := tar.NewReader(gz)

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		must(err)

		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(h.Name, "/mise") {
			continue
		}

		bin, err := os.OpenFile(misePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		must(err)

		_, err = io.Copy(bin, tr)
		must(err)
		must(bin.Close())

		return misePath
	}
	panic("binaire mise introuvable")
}

func installMise() string {
    dir := localBin()
	misePath := extractMiseFromURL(latestURL, dir)
	fmt.Println("mise installé dans", misePath)

	home, err := os.UserHomeDir()
	must(err)
	miseShims := filepath.Join(home, ".local", "share", "mise", "shims")

	// Injection des shims et de ~/.local/bin dans le PATH de Go
	newPath := fmt.Sprintf("%s:%s:%s", miseShims, dir, os.Getenv("PATH"))
	must(os.Setenv("PATH", newPath))

	return misePath
}

func readTools(jsonFile string) []string {
	file, err := os.Open(jsonFile)
	must(err)
	defer file.Close()

	var tools []string
	must(json.NewDecoder(file).Decode(&tools))

	return tools
}

type Tool struct {
	Name    string
	Version string
	URL     string
}

var bundles = map[string][]Tool{
	"forgejo": {
		{Name: "forgejo-runner", Version: "12.13", URL: "https://code.forgejo.org/forgejo/runner/releases/download/v12.13.0/forgejo-runner-12.13.0-linux-amd64"},
	    {Name: "forgejo", Version: "15.0.3", URL: "https://codeberg.org/forgejo/forgejo/releases/download/v15.0.3/forgejo-15.0.3-linux-amd64"},
	},
	"helm": {
		{Name: "helm", Version: "3.14.0"},
		{Name: "aqua:arttor/helmify", Version: "0.4.19"},
	},
	"kubectl": {
		{Name: "kubectl", Version: "1.29.0"},
		{Name: "kompose", Version: "1.38.0"},
	},
	"terraform": {
		{Name: "terraform", Version: "1.8.5"},
	},
	"k3s": {
		{Name: "k3s", Version: "1.35.3+k3s1"},
	},
	"docker": {
		{
			Name:    "docker",
			Version: "29.3.0",
			URL:     "https://download.docker.com/linux/static/stable/x86_64/docker-29.3.0.tgz",
		},
		{
			Name:    "dockerizer",
			Version: "1.0.0",
			URL:     "https://github.com/MelkiBenjamin/Cli/raw/refs/heads/main/my-artifact.zip",
		},
	},
}

func expand(tools []string) []Tool {
	var result []Tool

	for _, t := range tools {
		if bundle, ok := bundles[t]; ok {
			result = append(result, bundle...)
		}
	}
	return result
}

func prepaMise(tools []Tool) []string { // prépare la commande mise
	var args []string
	args = append(args, "use")
	for _, t := range tools {
		if t.URL == "" {
			args = append(args, t.Name+"@"+t.Version)
		} else {
			args = append(args,
				fmt.Sprintf("http:%s[url=%s]@%s", t.Name, t.URL, t.Version),
			)
		}
	}
	fmt.Println("Running:", args)
	return args
}

func hasTool(tools []Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func runShell(command string, args ...string) { // Pour lancer des commandes shell
    fullCommand := command
	if len(args) > 0 {
        fullCommand += " " + strings.Join(args, " ")
    }
	//fmt.Println("Avant commande:", fullCommand)
	cmd := exec.Command("sh", "-lc", fullCommand)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())
	//fmt.Println("Après commande:", fullCommand)
}

func runMise(misePath string, tools []Tool) { // pour installer les outils
	args := prepaMise(tools)
	runShell(misePath, args...)
}

var cmdDockerizer = `
    dockerizer . && \
    sed -i '1,5d' Dockerfile && \
    sed -i '1,3d' docker-compose.yml && \
	cp .env.example .env && \
    sed -i "s|.*build:.*|    image: app-prod|" docker-compose.yml && \
    sed -i '/context:/d; /dockerfile:/d' docker-compose.yml && \
	{ find . -name "*.go" -exec grep -qE "http\.ListenAndServe|http\.Serve|Listen\(" {} + || \
    sed -i -e "/EXPOSE/d" -e "/HEALTHCHECK/,+1d" Dockerfile; }
`

func startGenerate(tools []Tool) {
	if hasTool(tools, "docker") {
		runShell(cmdDockerizer)	// lance dockerizer.dev et corrige dockerfile 	
	}

	if hasTool(tools, "kompose") {
		runShell("cp .env.example .env")
		runShell("kompose convert") // lance kompose pour manifest k8s
	}

	if hasTool(tools, "helm") {
		runShell("kompose convert -c") // lance kompose pour helm chart
	}
	//WorflowsGit(AutoIsMicroservice())
}

func installAutoDocker(misePath string) []Tool {
    fmt.Println("🤖 Aucun Install.json. Lancement du mode automatique...")	// On récupère le bundle docker
	tools := append(bundles["forgejo"], bundles["docker"]...)

	runMise(misePath, tools)
	
	return tools
}

func AutoIsMicroservice() bool { // Regle pour vérifier si apli microservices 
	data, err := os.ReadFile("docker-compose.yml")
	return err == nil && strings.Count(string(data), "image:") > 1
}

func installAndGenerateK8s(misePath string) {
    fmt.Println("🏢 Architecture multiple détectée -> Passage à K8s")
    
    k8sTools := append(bundles["kubectl"], bundles["helm"]...)
    
    runMise(misePath, k8sTools)
    startGenerate(k8sTools)
}

func microservicesk8s(misePath string) {
    if AutoIsMicroservice() {
            installAndGenerateK8s(misePath)
    } else {
            fmt.Println("📦 Monolithe détecté -> On reste sur Docker Compose.")
    }
}

func startDaemon(logPath string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	logFile, err := os.Create(logPath)
	must(err)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	return cmd.Start()
}

func ConfigForgejo() (string) {
	fmt.Println("\n[*] --- Démarrage de Forgejo ---")
	home, _ := os.UserHomeDir()
	forgejoDir := filepath.Join(home, "forgejo")

	// Création explicite du dossier custom/conf et du fichier app.ini AVANT le démarrage
	confDir := filepath.Join(forgejoDir, "custom", "conf")
	must(os.MkdirAll(confDir, 0o755))
	appIniPath := filepath.Join(confDir, "app.ini")

	if _, err := os.Stat(appIniPath); os.IsNotExist(err) {
		initialConfig := fmt.Sprintf(`
[security]
INSTALL_LOCK = true

[database]
DB_TYPE = sqlite3
PATH    = %s

[repository]
ENABLE_PUSH_CREATE_USER = true
`, filepath.Join(forgejoDir, "data", "forgejo.db"))

		must(os.WriteFile(appIniPath, []byte(initialConfig), 0o644))
		fmt.Println("[+] Fichier app.ini initialisé.")
	}

	// Démarrage du démon Forgejo
	fmt.Println("[*] Démarrage du démon Forgejo...")
	must(startDaemon("forgejo.log", "forgejo", "web", "--work-path", forgejoDir))

	for i := 0; i < 10; i++ {
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:3000", 500*time.Millisecond); err == nil {
		conn.Close()
		break
	}
	time.Sleep(500 * time.Millisecond)
    }

	fmt.Println("[+] Forgejo est prêt sur http://localhost:3000.")
	return forgejoDir
}

func ConfigRunner(forgejoDir string) string {
	fmt.Println("\n[*] --- Configuration Déclarative du Runner CI/CD ---")

	// 1. Génération d'un secret hexadécimal de 40 caractères
	secretBytes := make([]byte, 20)
	_, err := crand.Read(secretBytes)
	must(err)
	sharedSecret := hex.EncodeToString(secretBytes)

	// 2. Enregistrement côté Forgejo (serveur CLI) et récupération de l'UUID
	cmd := exec.Command("forgejo", "forgejo-cli", "actions", "register",
		"--name", "runner-zero-touch",
		"--secret", sharedSecret,
		"--work-path", forgejoDir)

    out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("Erreur lors de l'enregistrement: %s\n", string(out))
		must(err)
	}
	runnerUUID := strings.TrimSpace(string(out))

	fmt.Printf("[+] Runner enregistré. UUID : %s\n", runnerUUID)

	configPath := filepath.Join(forgejoDir, "runner-config.yaml")

	// 3. Écriture du fichier de configuration automatique .forgejo-runner
	runnerConfig := fmt.Sprintf(`
log: 
  level: debug
runner:
  capacity: 1
  labels:
    - "self-hosted:host"
server:
  connections:
    local-forgejo:
      url: "http://localhost:3000"
      uuid: "%s"
      token: "%s"
`, runnerUUID, sharedSecret)

	must(os.WriteFile(configPath, []byte(runnerConfig), 0o600))
	fmt.Printf("[+] Configuration .forgejo-runner générée : %s\n", configPath)

	fmt.Println("\n[*] Démarrage du runner Forgejo (logs redirigés dans runner.log)...")
	must(startDaemon("runner.log", "forgejo-runner", "daemon", "-c", configPath))

	return configPath
}

func createAdmin(forgejoDir string) (string, string) {
	fmt.Println("\n[*] --- Création de user administrateur")

	userBytes := make([]byte, 8)
    passBytes := make([]byte, 16)
    _, _ = crand.Read(userBytes)
    _, _ = crand.Read(passBytes)

    adminUser := "admin_" + hex.EncodeToString(userBytes)
    adminPass := hex.EncodeToString(passBytes)
	adminEmail := adminUser + "@localhost"
	
	runShell("forgejo", "admin", "user", "create",
		"--username", adminUser,
		"--password", adminPass,
		"--email", adminEmail,
		"--admin",
		"--work-path", forgejoDir)

	return adminUser, adminPass
}

func WorflowsGit(tools []Tool) {
	fmt.Println("\n[*] --- Génération worflows Git ---")
	must(os.MkdirAll(".github/workflows", 0o755))
	isMicro := false
	if _, err := os.Stat("Install.json"); err == nil {
		// Mode Expert : on regarde les outils déclarés
		isMicro = hasTool(tools, "kubectl") || hasTool(tools, "k3s") || hasTool(tools, "helm")
	} else {
		// Mode Automatique : on se base sur la structure du compose généré
		isMicro = AutoIsMicroservice()
	}

	workflowContent := `name: CI/CD Pipeline
on: [push]
jobs:
  build-deploy:
    runs-on: self-hosted
    steps:
    - uses: actions/checkout@v7.0.1
    - name: Build Docker Image
      run: docker build --network=host --progress=plain -t app-prod .
`
	if isMicro {
		workflowContent += `
    - name: Deploy Kubernetes
      run: helm install test ./docker-compose
`
	} else {
		workflowContent += `
    - name: Deploy Docker Compose
      run: docker compose --progress=plain up -d 
`
	}
	must(os.WriteFile(".github/workflows/main.yaml", []byte(workflowContent), 0o644))
}

func configGit(user, password string) {
	_ = os.RemoveAll(".git")
	fmt.Println("Config de Git...")
	runShell("git config --global init.defaultBranch main")
	runShell("git init")
	runShell("git config user.name '" + user + "'")
	runShell("git config user.email '" + user + "@localhost'")
	runShell("git config transfer.credentialsInUrl allow")
    // création de dépôt a la volée avec fonction push to create Forgejo 
	remoteURL := fmt.Sprintf("http://%s:%s@localhost:3000/%s/app-repo.git", user, password, user)
	runShell("git remote remove origin || true")
	runShell("git remote add origin " + remoteURL)
}

func gitAddCommitPush() {
	runShell("git add .")
	runShell("git commit -m 'Zero-Touch: Auto-generated pipeline'")
	runShell("git push -u origin main --force")
	fmt.Println("[+] Pipeline GitOps déployé !")
}

func stepInstallAndConfig(misePath string) []Tool {
	fmt.Println("\n=== ÉTAPE 1 : INSTALLATION ET CONFIGURATION ===")
	var tools []Tool
	if _, err := os.Stat("Install.json"); err == nil {
		// Mode Expert
		tools = append(bundles["forgejo"], expand(readTools("Install.json"))...)
		runMise(misePath, tools)
		if hasTool(tools, "k3s") {
			fmt.Println("[*] Mode Expert : Démarrage de k3s server...")
			configK3sRootless() 
		}
	} else {
		// Mode Automatique
		tools = installAutoDocker(misePath)
	}
	forgejoDir := ConfigForgejo()
	adminUser, adminPass := createAdmin(forgejoDir)
	ConfigRunner(forgejoDir)
	configGit(adminUser, adminPass)

	return tools
}

func stepGenerate(misePath string, tools []Tool) {
	fmt.Println("\n=== ÉTAPE 2 : GÉNÉRATION DES ARTÉFACTS ===")
	startGenerate(tools)
	if _, err := os.Stat("Install.json"); err != nil {
		microservicesk8s(misePath)
	}
	WorflowsGit(tools)

}

func configK3sRootless1() error {
	homeDir, err := os.UserHomeDir()
	must(err)

	fmt.Println("[*] Configuration d'AppArmor pour K3s Rootless...")
    miseK3sBin := filepath.Join(homeDir, ".local", "share", "mise", "installs", "k3s", "1.35.3+k3s1", "k3s")

	// Création et application du profil AppArmor officiel
	appArmorConfig := fmt.Sprintf(`abi <abi/4.0>,
include <tunables/global>

%s flags=(unconfined) {
  userns,
}`, miseK3sBin)
	
	profileFileName := "home.runner..local.share.mise.installs.k3s.1.35.3+k3s1.k3s"
	cmdEcho := fmt.Sprintf("echo '%s' | sudo tee /etc/apparmor.d/%s > /dev/null", appArmorConfig, profileFileName)
	
	runShell(cmdEcho)
	runShell("sudo systemctl restart apparmor.service")

	// 2. CONFIGURATION DES CGROUPS v2 (Délégation systemd pour le cpuset)
	runShell("sudo mkdir -p /etc/systemd/system/user@.service.d")
	delegateConfig := `[Service]
Delegate=cpu memory io cpuset pids`
	
	cmdDelegate := fmt.Sprintf("echo '%s' | sudo tee /etc/systemd/system/user@.service.d/delegate.conf > /dev/null", delegateConfig)
	runShell(cmdDelegate)
	runShell("sudo systemctl daemon-reload")
	runShell(fmt.Sprintf("sudo systemctl restart user@%d.service", os.Getuid()))

	logPath := filepath.Join(homeDir, "k3s.log")
	
	systemdUserDir := filepath.Join(homeDir, ".config", "systemd", "user")
	servicePath := filepath.Join(systemdUserDir, "k3s-rootless.service")

	must(os.MkdirAll(systemdUserDir, 0755))

	// Astuce : on encapsule l'appel dans un shell (`/bin/sh -c`) pour rediriger stdout/stderr 
	// directement dans le fichier de log, contournant les limitations de systemd sur les runners CI.
	k3sCmd := fmt.Sprintf("/bin/sh -c '%s server --rootless --snapshotter=fuse-overlayfs >> %s 2>&1'", miseK3sBin, logPath)

	serviceContent := fmt.Sprintf(`[Unit]
Description=K3s Rootless
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStart=%s
Delegate=yes
KillMode=mixed
LimitNOFILE=infinity
LimitNPROC=infinity
LimitCORE=infinity
TasksMax=infinity
StartLimitBurst=3
StartLimitInterval=60s
TimeoutStartSec=0
Restart=always
RestartSec=2
TimeoutSec=0

[Install]
WantedBy=default.target
`, k3sCmd)

	must(os.WriteFile(servicePath, []byte(serviceContent), 0644))

	// Forcer l'export de XDG_RUNTIME_DIR au cas où pour systemctl
	os.Setenv("XDG_RUNTIME_DIR", fmt.Sprintf("/run/user/%d", os.Getuid()))

	runShell("systemctl", "--user", "daemon-reload")
	runShell("systemctl", "--user", "enable", "--now", "k3s-rootless")

	// Pause pour laisser K3s s'initialiser ou crasher
	time.Sleep(5 * time.Second)

	// Lecture directe et infaillible du fichier de log physique !
	if logBytes, err := os.ReadFile(logPath); err == nil && len(logBytes) > 0 {
		fmt.Printf("--- CONTENU DE K3S.LOG ---\n%s\n", string(logBytes))
	} else {
		fmt.Println("[!] Le fichier k3s.log est vide ou introuvable.")
	}

	// Statut final
	cmdStatus := exec.Command("systemctl", "--user", "status", "k3s-rootless")
	output, _ := cmdStatus.CombinedOutput()
	fmt.Printf("--- Statut du service K3s Rootless ---\n%s\n", string(output))

    os.Setenv("KUBECONFIG", filepath.Join(homeDir, ".kube", "k3s.yaml"))
	
	return nil
}

func configK3sRootless() error {
	homeDir, err := os.UserHomeDir()
	must(err)

	fmt.Println("[*] Démarrage de K3s Rootless via systemd-run...")

	miseK3sBin := filepath.Join(homeDir, ".local", "share", "mise", "installs", "k3s", "1.35.3+k3s1", "k3s")
	logPath := filepath.Join(homeDir, "k3s.log")

	// Création et application du profil AppArmor officiel
	appArmorConfig := fmt.Sprintf(`abi <abi/4.0>,
include <tunables/global>

%s flags=(unconfined) {
  userns,
}`, miseK3sBin)
	
    profileFileName := strings.ReplaceAll(strings.TrimPrefix(miseK3sBin, "/"), "/", ".")
	cmdEcho := fmt.Sprintf("echo '%s' | sudo tee /etc/apparmor.d/%s > /dev/null", appArmorConfig, profileFileName)
	
	runShell(cmdEcho)
	runShell("sudo systemctl restart apparmor.service")

	// 2. CONFIGURATION DES CGROUPS v2 (Délégation systemd pour le cpuset)
	runShell("sudo mkdir -p /etc/systemd/system/user@.service.d")
	delegateConfig := `[Service]
Delegate=cpu memory io cpuset pids`
	
	cmdDelegate := fmt.Sprintf("echo '%s' | sudo tee /etc/systemd/system/user@.service.d/delegate.conf > /dev/null", delegateConfig)
	runShell(cmdDelegate)
	runShell("sudo systemctl daemon-reload")
	runShell(fmt.Sprintf("sudo systemctl restart user@%d.service", os.Getuid()))
	
	// Forcer XDG_RUNTIME_DIR pour systemd utilisateur
	os.Setenv("XDG_RUNTIME_DIR", fmt.Sprintf("/run/user/%d", os.Getuid()))

	// Préparation du fichier de log
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	//if err != nil {
	//	return fmt.Errorf("impossible de créer le fichier de log : %w", err)
	//}
	defer logFile.Close()

	// Lancement via systemd-run avec --no-block (OBLIGATOIRE pour un serveur)
	cmd := exec.Command("systemd-run", "--user",
		"--unit=k3s-rootless",
		"--no-block", // <-- Empêche Go de bloquer en attendant la fin de K3s
		"-p", "Delegate=yes",
		"-p", "KillMode=mixed",
		"-p", "Restart=always",
		"-p", "RestartSec=2",
		miseK3sBin, "server", "--rootless", "--snapshotter=fuse-overlayfs",
	)

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("échec du lancement de systemd-run : %w", err)
	}

	fmt.Println("[*] Service enregistré. Attente de l'initialisation de K3s (5 secondes)...")
	time.Sleep(20 * time.Second)

	// RETOUR DE L'AFFICHAGE DES LOGS pour voir si K3s démarre bien ou s'il plante
	if logBytes, err := os.ReadFile(logPath); err == nil && len(logBytes) > 0 {
		fmt.Printf("--- CONTENU DE K3S.LOG ---\n%s\n", string(logBytes))
	} else {
		fmt.Println("[!] Le fichier de log est vide ou inaccessible pour le moment.")
	}

	// Affichage du statut systemd utilisateur
	//statusCmd := exec.Command("systemctl", "--user", "status", "k3s-rootless")
	runShell("systemctl", "--user", "status", "k3s-rootless")
	//output, _ := statusCmd.CombinedOutput()
	//fmt.Printf("--- Statut du service K3s Rootless ---\n%s\n", string(output))

	// Configuration de KUBECONFIG
	os.Setenv("KUBECONFIG", filepath.Join(homeDir, ".kube", "k3s.yaml"))
	//fmt.Println("[*] Orchestration terminée !")
	//fmt.Println("[*] Service enregistré. Vérification des logs systemd...")
	//time.Sleep(3 * time.Second)

	// Récupérer les 50 dernières lignes des logs de l'unité systemd utilisateur
	//logCmd := exec.Command("journalctl", "--user", "-u", "k3s-rootless", "-n", "50", "--no-pager")
	runShell("journalctl", "--user", "-u", "k3s-rootless", "-n", "150", "--no-pager")
	//logOutput, err := logCmd.CombinedOutput()
	//if err != nil {
	//	fmt.Printf("[!] Impossible de lire les logs journalctl : %v\n", err)
	//} else {
	//	fmt.Printf("--- VRAIS LOGS DE K3S (Journalctl) ---\n%s\n", string(logOutput))
	//}

	return nil
}

func main() { 
	// Headless IDP : prepare, generate and publish a deployable project.
	// Étape 1 : prerequis, install et config outils selon 2 modes "expert" (outils dans json+forgejo) ou "automatique" (outils docker fogejo et selon regle k8s ou non)
    misePath := installMise()
    tools := stepInstallAndConfig(misePath)
	// Étape 2 : principale metier DevOps Generation des yaml et fichier DevOps (dockerfile, docker compose, manifest k8s, helm, workflows actions git)
    stepGenerate(misePath, tools)
	// Étape 3 : deploiement Push
	gitAddCommitPush()

	fmt.Println("\n[🎉] Chaîne complète exécutée avec succès !")
}
