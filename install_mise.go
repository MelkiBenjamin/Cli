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
	"bytes"
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
	fmt.Println("Avant commande:", fullCommand)
	cmd := exec.Command("sh", "-lc", `export PATH="$HOME/.local/bin:$PATH" && eval "$(mise activate bash --shims)" && `+fullCommand)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())
	fmt.Println("Après commande:", fullCommand)
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
}

func installAutoDocker(misePath string) []Tool {
    fmt.Println("🤖 Aucun Install.json. Lancement du mode automatique...")	// On récupère le bundle docker
	tools := bundles["docker"]
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

func startMode(misePath string) {
	if _, err := os.Stat("Install.json"); err == nil {
		// --- MODE 1 : EXPERT ---
		tools := readTools("Install.json") // lecture du json
		expanded := expand(tools)
		runMise(misePath, expanded) // install des outils du json
		startGenerate(expanded)     // lancement des outils générateur
	} else {
		// --- MODE 2 : AUTOMATIQUE ---
		dockerTools := installAutoDocker(misePath) // install de docker dockerizer
		startGenerate(dockerTools) // lancement des outils générateur
        microservicesk8s(misePath) // inspecte si microservices et si oui, install outils k8s et lance générateur 
	}
}

// ============================================================================
// --- AJOUTS : CONSTANTES & FONCTIONS D'EXÉCUTION (EX-PYTHON) ---
// ============================================================================

const (
	forgejoURL = "https://codeberg.org/forgejo/forgejo/releases/download/v15.0.3/forgejo-15.0.3-linux-amd64"
	runnerURL  = "https://code.forgejo.org/forgejo/runner/releases/download/v12.13.0/forgejo-runner-12.13.0-linux-amd64"
)

func downloadFile(url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	must(os.MkdirAll(filepath.Dir(dest), 0o755))

	resp, err := http.Get(url)
	must(err)
	defer resp.Body.Close()

	// Intercepte les pages 404 / redirections HTML avant d'écrire le fichier
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("Échec téléchargement HTTP %d pour : %s", resp.StatusCode, url))
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	must(err)
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	must(err)

	// S'assure que les droits d'exécution +x sont bien appliqués
	return os.Chmod(dest, 0o755)
}

func startDaemon(logPath string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	logFile, err := os.Create(logPath)
	must(err)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	return cmd.Start()
}

func setupForgejo() (string, string) {
	fmt.Println("\n[*] --- Démarrage de Forgejo ---")
	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, ".local", "bin")
	forgejoBin := filepath.Join(binDir, "forgejo")
	forgejoDir := filepath.Join(home, "forgejo")

	if _, err := os.Stat(forgejoBin); err != nil {
		fmt.Println("[*] Téléchargement du binaire Forgejo...")
		must(downloadFile(forgejoURL, forgejoBin))
	}

	// Création explicite du dossier custom/conf et du fichier app.ini AVANT le démarrage
	confDir := filepath.Join(forgejoDir, "custom", "conf")
	must(os.MkdirAll(confDir, 0o755))
	appIniPath := filepath.Join(confDir, "app.ini")

	if _, err := os.Stat(appIniPath); os.IsNotExist(err) {
		initialConfig := fmt.Sprintf(`[DEFAULT]
RUN_MODE = prod

[server]
HTTP_PORT = 3000
ROOT_URL  = http://localhost:3000/
DOMAIN    = localhost
HTTP_ADDR = 127.0.0.1

[security]
INSTALL_LOCK = true

[database]
DB_TYPE = sqlite3
PATH    = %s

[actions]
ENABLED = true
`, filepath.Join(forgejoDir, "data", "forgejo.db"))

		must(os.WriteFile(appIniPath, []byte(initialConfig), 0o644))
		fmt.Println("[+] Fichier app.ini initialisé avec INSTALL_LOCK = true et [actions] ENABLED = true")
	}

	// Démarrage du démon Forgejo
	fmt.Println("[*] Démarrage du démon Forgejo...")
	must(startDaemon("forgejo.log", forgejoBin, "web", "--work-path", forgejoDir))

	for i := 0; i < 10; i++ {
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:3000", 500*time.Millisecond); err == nil {
		conn.Close()
		break
	}
	time.Sleep(500 * time.Millisecond)
    }

	fmt.Println("[+] Forgejo est prêt sur http://localhost:3000.")
	return forgejoBin, forgejoDir
}

func setupRunner(forgejoBin, forgejoDir string) string {
	fmt.Println("\n[*] --- Configuration Déclarative du Runner CI/CD ---")
	home, _ := os.UserHomeDir()
	runnerBin := filepath.Join(home, ".local", "bin", "forgejo-runner")

	if _, err := os.Stat(runnerBin); err != nil {
		fmt.Println("[*] Téléchargement du binaire Forgejo Runner...")
		must(downloadFile(runnerURL, runnerBin))
		must(os.Chmod(runnerBin, 0o755))
	}

	// 1. Génération d'un secret hexadécimal de 40 caractères
	secretBytes := make([]byte, 20)
	_, err := crand.Read(secretBytes)
	must(err)
	sharedSecret := hex.EncodeToString(secretBytes)

	// 2. Enregistrement côté Forgejo (serveur CLI) et récupération de l'UUID
	cmdRegister := exec.Command(forgejoBin, "forgejo-cli", "actions", "register",
		"--name", "runner-zero-touch",
		"--secret", sharedSecret,
		"--work-path", forgejoDir)

	out, err := cmdRegister.Output()
	must(err)
	runnerUUID := strings.TrimSpace(string(out))

	fmt.Printf("[+] Runner enregistré. UUID : %s\n", runnerUUID)

	configPath := filepath.Join(forgejoDir, "runner-config.yaml")

	// 3. Écriture du fichier de configuration automatique .forgejo-runner
	runnerConfig := fmt.Sprintf(`
log: 
  level: debug
runner:
  capacity: 1
  timeout: 3h
  shutdown_timeout: 0s
  fetch_timeout: 5s
  fetch_interval: 10s
  labels:
    - "self-hosted:host"
  envs:
    ACTIONS_RUNNER_FORCE_RETRY: "false"
host:
  workdir_parent: "/tmp/forgejo_runner_work"
server:
  connections:
    local-forgejo:
      url: "http://localhost:3000"
      uuid: "%s"
      token: "%s"
`, runnerUUID, sharedSecret)

	must(os.WriteFile(configPath, []byte(runnerConfig), 0o600))
	fmt.Println("[+] Configuration .forgejo-runner générée : %s\n", configPath)

	debugRunnerProcesses()

	return configPath
}

func runRunnerDaemon(configPath string) *exec.Cmd {
	fmt.Println("\n[*] Démarrage du runner Forgejo (logs redirigés dans runner.log)...")
	home, _ := os.UserHomeDir()
	runnerBin := filepath.Join(home, ".local", "bin", "forgejo-runner")

	// Fichier de log dédié au lieu de la console
	logFile, err := os.Create("runner.log")
	must(err)
    debugRunnerProcesses()
	
	cmdDaemon := exec.Command(runnerBin, "daemon", "-c", configPath)
//	cmdDaemon.Stdout = logFile // On jette Stdout pour ne pas avoir de doublon
//	cmdDaemon.Stderr = io.Discard  // Stderr contient TOUT (logs + debug + erreurs)
	
	debugRunnerProcesses()

	// Start() lance le daemon en arrière-plan au lieu de tout bloquer
	must(cmdDaemon.Start())
	
	debugRunnerProcesses()

	return cmdDaemon
}

func waitForJobCompletion(user, pass string) {
	fmt.Println("[*] En attente de la fin du pipeline CI/CD...")
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:3000/api/v1/repos/%s/app-repo/actions/runs", user)

	for i := 0; i < 60; i++ { // Attend jusqu'à 2 minutes max
		time.Sleep(2 * time.Second)

		req, _ := http.NewRequest("GET", url, nil)
		req.SetBasicAuth(user, pass)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		var result struct {
			WorkflowRuns []struct {
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			} `json:"workflow_runs"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && len(result.WorkflowRuns) > 0 {
			run := result.WorkflowRuns[0]
			if run.Status == "completed" {
				fmt.Printf("[+] Pipeline terminé (Statut : %s)\n", run.Conclusion)
				resp.Body.Close()
				return
			}
		}
		resp.Body.Close()
	}
}

func createAdminAndRepo(forgejoBin, forgejoDir string) (string, string) {
	fmt.Println("\n[*] --- Création de l'administrateur et du dépôt ---")

	userBytes := make([]byte, 8)
    passBytes := make([]byte, 16)
    _, _ = crand.Read(userBytes)
    _, _ = crand.Read(passBytes)

    adminUser := "admin_" + hex.EncodeToString(userBytes)
    adminPass := hex.EncodeToString(passBytes)
	adminEmail := adminUser + "@localhost"
	
	cmd := exec.Command(forgejoBin, "admin", "user", "create",
		"--username", adminUser,
		"--password", adminPass,
		"--email", adminEmail,
		"--admin",
		"--work-path", forgejoDir)
	_ = cmd.Run()

	repoName := "app-repo"
	reqBody, _ := json.Marshal(map[string]interface{}{
		"name":    repoName,
		"private": false,
	})

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", "http://127.0.0.1:3000/api/v1/user/repos", bytes.NewBuffer(reqBody))
	req.SetBasicAuth(adminUser, adminPass)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		fmt.Printf("[+] Compte '%s' et dépôt '%s' créés.\n", adminUser, repoName)
	}

	return adminUser, adminPass
}

func deployGitOps(isMicroservice bool, user, password string) {
	fmt.Println("\n[*] --- Génération CI/CD et Déploiement Git ---")
	must(os.MkdirAll(".github/workflows", 0o755))

	workflowContent := `name: CI/CD Pipeline
on: [push]
jobs:
  build-deploy:
    runs-on: self-hosted
    steps:
    - uses: actions/checkout@v4
    - name: Build Docker Image
      run: docker build --network=host --progress=plain -t app-prod .
`
	if isMicroservice {
		workflowContent += `
    - name: Deploy Kubernetes
      run: kubectl apply -f k8s/
`
	} else {
		workflowContent += `
    - name: Deploy Docker Compose
      run: GITHUB_SHA=${{ github.sha }} docker compose --progress=plain up -d 
`
	}

	must(os.WriteFile(".github/workflows/main.yaml", []byte(workflowContent), 0o644))

	_ = os.RemoveAll(".git")

	// Initialisation avec la branche 'main' explicitement
	runShell("git config --global init.defaultBranch main")
	runShell("git init")
	runShell("git config user.name '" + user + "'")
	runShell("git config user.email '" + user + "@localhost'")
	runShell("git config transfer.credentialsInUrl allow")

	remoteURL := fmt.Sprintf("http://%s:%s@localhost:3000/%s/app-repo.git", user, password, user)
	runShell("git remote remove origin || true")
	runShell("git remote add origin " + remoteURL)

	runShell("git add .")
	runShell("git commit -m 'Zero-Touch: Auto-generated pipeline'")
	runShell("git push -u origin main --force")
	fmt.Println("[+] Pipeline GitOps déployé !")
	debugRunnerProcesses()
}

// Structure minimale pour lire la réponse de l'API Forgejo
type ActionRunsResponse struct {
	TotalCount int `json:"total_count"`
	Runs       []struct {
		ID     int64  `json:"id"`
		Event  string `json:"event"`
		Status string `json:"status"`
	} `json:"workflow_runs"`
}

func debugRunnerProcesses() {
	fmt.Println("\n========== DEBUG FORGEJO RUNNER ==========")

	cmd := exec.Command("sh", "-lc",
		`ps -eo pid,ppid,lstart,args | grep '[f]orgejo-runner' || true`,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Println("[DEBUG] erreur ps:", err)
	}

	fmt.Println("==========================================")
}

func checkForgejoRunsCount(user, password string) {
	// Petite pause de 1-2s pour laisser à Forgejo le temps d'enregistrer l'événement Git
	time.Sleep(2 * time.Second)

	url := fmt.Sprintf("http://localhost:3000/api/v1/repos/%s/app-repo/actions/runs", user)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("[DEBUG] Erreur création requête API : %v\n", err)
		return
	}

	req.SetBasicAuth(user, password)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[DEBUG] Erreur appel API Forgejo : %v\n", err)
		return
	}
	defer resp.Body.Close()

	var data ActionRunsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Printf("[DEBUG] Erreur décodage JSON API : %v\n", err)
		return
	}

	fmt.Printf("----------------------------------------\n")
	fmt.Printf("[DIAGNOSTIC API] Nombre de runs détectés dans Forgejo : %d\n", len(data.Runs))
	for i, run := range data.Runs {
		fmt.Printf(" -> Run #%d : ID=%d | Event=%s | Status=%s\n", i+1, run.ID, run.Event, run.Status)
	}
	fmt.Printf("----------------------------------------\n")
}


func main() {
	// Étape 1 : Préparer l'exécutable 'mise' (Téléchargement + Extraction)
    misePath := installMise()
    // Étape 2 : Décider s'il faut utiliser le mode avec JSON (Expert) ou mode de l'Auto-détection (Automatique)
    startMode(misePath)
	// étape 3
	isMicro := AutoIsMicroservice()
	forgejoBin, forgejoDir := setupForgejo()
	adminUser, adminPass := createAdminAndRepo(forgejoBin, forgejoDir)
	configPath := setupRunner(forgejoBin, forgejoDir)
	cmdDaemon := runRunnerDaemon(configPath)
	
    // 2. Push GitOps
	time.Sleep(2 * time.Second)
	deployGitOps(isMicro, adminUser, adminPass)
    checkForgejoRunsCount(adminUser, adminPass)
	
	// 3. Attente du résultat du pipeline
	waitForJobCompletion(adminUser, adminPass)
	checkForgejoRunsCount(adminUser, adminPass)
	fiBefore, _ := os.Stat("runner.log")
    fmt.Printf("\n[DEBUG] Taille runner.log AVANT arrêt du daemon : %d octets\n", fiBefore.Size())

	/// 4. On arrête le daemon du runner pour fermer le script Go
	if cmdDaemon != nil && cmdDaemon.Process != nil {
		fmt.Println("\n[INFO] Arrêt propre du runner...")
		
		// Envoi de SIGINT (Ctrl+C) pour une fermeture propre des buffers
		if err := cmdDaemon.Process.Signal(os.Interrupt); err != nil {
			_ = cmdDaemon.Process.Kill()
		} else {
			// canal pour attendre la fin du process
			done := make(chan error, 1)
			go func() {
				done <- cmdDaemon.Wait()
			}()

			// On laisse 3 secondes max au runner pour se fermer proprement
			select {
			case <-time.After(3 * time.Second):
				fmt.Println("[WARN] Le runner ne répond pas, arrêt forcé.")
				_ = cmdDaemon.Process.Kill()
			case <-done:
				fmt.Println("[INFO] Runner arrêté et fichiers de logs fermés.")
			}
		}
	}
	fiBefore, _ = os.Stat("runner.log")
    fmt.Printf("\n[DEBUG] Taille runner.log APRÈS arrêt du daemon : %d octets\n", fiBefore.Size())
	checkForgejoRunsCount(adminUser, adminPass)
	fmt.Println("\n[🎉] Chaîne complète exécutée avec succès !")
	checkForgejoRunsCount(adminUser, adminPass)
}
