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
	"crypto/rand"
	"encoding/hex"
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

//func workflows(
//	docker build
//	if hasTool(tools, "docker") {
//		runShell(docker compose up)	//lance docker compose
//	}
//	if hasTool(tools, "kubectl) {
//		runShell(kubectl -f .)	//lance manifest k8s
//	}

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

func generateRandomSecret(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func downloadFile(url, dest string, minSize int64) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	must(os.MkdirAll(filepath.Dir(dest), 0o755))
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}

	n, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return err
	}

	if minSize > 0 && n < minSize {
		os.Remove(tmp)
		return fmt.Errorf("fichier trop petit : %d octets", n)
	}

	return os.Rename(tmp, dest)
}

func waitForPort(host string, port int, timeout time.Duration) bool {
	target := fmt.Sprintf("%s:%d", host, port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", target, 2*time.Second)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return false
}

func startDaemon(logPath string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
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
		must(downloadFile(forgejoURL, forgejoBin, 50*1024*1024))
	}

	_ = exec.Command("pkill", "-9", "-f", "forgejo").Run()
	time.Sleep(1 * time.Second)

	// Création explicite du dossier custom/conf et du fichier app.ini AVANT le démarrage
	confDir := filepath.Join(forgejoDir, "custom", "conf")
	must(os.MkdirAll(confDir, 0o755))
	appIniPath := filepath.Join(confDir, "app.ini")

	providedAppIni := filepath.Join("configs", "forgejo", "app.ini")
	if _, err := os.Stat(providedAppIni); err != nil {
		panic("Fichier requis absent : configs/forgejo/app.ini")
	}
	input, err := os.ReadFile(providedAppIni)
	must(err)
	must(os.WriteFile(appIniPath, input, 0o644))
		fmt.Println("[+] Fichier app.ini initialisé avec INSTALL_LOCK = true et [actions] ENABLED = true")
	}

	// Démarrage du démon Forgejo
	fmt.Println("[*] Démarrage du démon Forgejo...")
	must(startDaemon("forgejo.log", forgejoBin, "web", "--work-path", forgejoDir))

	if !waitForPort("127.0.0.1", 3000, 60*time.Second) {
		panic("Forgejo ne répond pas sur le port 3000")
	}

	fmt.Println("[+] Forgejo est prêt sur http://localhost:3000.")
	return forgejoBin, forgejoDir
}
// Lit un fichier INI et vérifie si une clé sous une section a une valeur spécifique
func checkIniValue(filePath, section, key, expectedValue string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentSection := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Ignorer commentaires et lignes vides
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Détection de section [section]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}

		// Traitement clé = valeur dans la bonne section
		if strings.EqualFold(currentSection, section) && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])

			if strings.EqualFold(k, key) {
				return strings.EqualFold(v, expectedValue)
			}
		}
	}
	return false
}

func setupRunner(forgejoBin, forgejoDir string) {
	fmt.Println("\n[*] --- Configuration du Runner CI/CD ---")
	home, _ := os.UserHomeDir()
	runnerBin := filepath.Join(home, ".local", "bin", "forgejo-runner")

	// 0. Téléchargement du binaire Runner si absent
	if _, err := os.Stat(runnerBin); err != nil {
		fmt.Println("[*] Téléchargement du binaire Forgejo Runner...")
		must(downloadFile(runnerURL, runnerBin, 10*1024*1024))
		must(os.Chmod(runnerBin, 0o755))
	}

	appIniPath := filepath.Join(forgejoDir, "custom", "conf", "app.ini")
	fmt.Printf("[🔍] Inspection du fichier INI : %s\n", appIniPath)

	// 1. Vérification avec notre lecteur INI
	actionsEnabled := checkIniValue(appIniPath, "actions", "ENABLED", "true")
	if actionsEnabled {
		fmt.Println("  └─ [OK] Section [actions] ENABLED = true confirmée dans app.ini")
	} else {
		fmt.Println("  └─ [⚠️] [actions] non activé dans app.ini. Correction en cours...")
		
		// Injecter [actions] si absent
		f, err := os.OpenFile(appIniPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		must(err)
		_, _ = f.WriteString("\n[actions]\nENABLED = true\n")
		f.Close()

		// Redémarrer Forgejo pour charger la nouvelle configuration
		fmt.Println("[*] Redémarrage de Forgejo pour appliquer la configuration...")
		_ = exec.Command("pkill", "-9", "-f", "forgejo").Run()
		time.Sleep(1 * time.Second)
		must(startDaemon("forgejo.log", forgejoBin, "web", "--work-path", forgejoDir))
		if !waitForPort("127.0.0.1", 3000, 30*time.Second) {
			panic("Forgejo ne répond pas après redémarrage")
		}
	}

	// 2. Génération du Token via la CLI
	var runnerToken string
	var lastErr string

	for i := 0; i < 10; i++ {
		cmd := exec.Command(forgejoBin, "actions", "generate-runner-token", "--config", appIniPath, "--work-path", forgejoDir)
		cmd.Dir = forgejoDir
		out, err := cmd.CombinedOutput()

		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if len(line) >= 32 && !strings.Contains(line, " ") && !strings.Contains(line, "[") {
					runnerToken = line
					break
				}
			}
			if runnerToken != "" {
				break
			}
		}
		lastErr = string(out)
		time.Sleep(2 * time.Second)
	}

	if runnerToken == "" {
		fmt.Printf("[❌] Échec CLI Forgejo.\nSortie brute :\n%s\n", lastErr)
		panic("Impossible de récupérer le token du Runner")
	}

	fmt.Printf("[+] Token Runner récupéré : %s...\n", runnerToken[:8])

	// 3. Enregistrement et Démarrage du Runner
	configDir := filepath.Join(home, ".runner_config")
	must(os.MkdirAll(configDir, 0o755))
	configFile := filepath.Join(configDir, "config.yaml")

	configContent := fmt.Sprintf(`
log:
  level: debug
runner:
  capacity: 1
  name: runner-zero-touch
  envs: {}
  timeout: 3h
  shutdown_timeout: 0s
  fetch_timeout: 5s
  fetch_interval: 2s
  labels:
    - "self-hosted:host"
  host:
    workdir_parent: "%s"
`, filepath.Join(configDir, "workdir"))

	providedRunnerConf := filepath.Join("configs", "runner", "config.yaml")
	if _, err := os.Stat(providedRunnerConf); err != nil {
		panic("Fichier requis absent : configs/runner/config.yaml")
	}
	input, err := os.ReadFile(providedRunnerConf)
	must(err)
	must(os.WriteFile(configFile, input, 0o644))

	_ = os.Remove(filepath.Join(configDir, ".runner"))

	regCmd := exec.Command(runnerBin, "register",
		"--instance", "http://localhost:3000",
		"--token", runnerToken,
		"--name", "runner-zero-touch",
		"--no-interactive",
		"--config", configFile)

	regCmd.Dir = configDir
	out, err := regCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("[❌] Erreur d'enregistrement du Runner : %s\n", string(out))
		panic(err)
	}

	_ = exec.Command("pkill", "-9", "-f", "forgejo-runner").Run()

	cmdDaemon := exec.Command(runnerBin, "daemon", "--config", configFile)
	cmdDaemon.Dir = configDir

	logFile, err := os.Create("runner.log")
	must(err)
	cmdDaemon.Stdout = logFile
	cmdDaemon.Stderr = logFile

	must(cmdDaemon.Start())
	fmt.Println("[+] Runner CI/CD démarré.")
}

func createAdminAndRepo(forgejoBin, forgejoDir string) (string, string) {
	fmt.Println("\n[*] --- Création de l'administrateur et du dépôt ---")

	adminUser := "admin_" + generateRandomSecret(3)
	adminPass := generateRandomSecret(10)
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
      run: docker build --network=host -t app-prod:${{ github.sha }} .
`
	if isMicroservice {
		workflowContent += `
    - name: Deploy Kubernetes
      run: kubectl apply -f k8s/
`
	} else {
		workflowContent += `
    - name: Deploy Docker Compose
      run: docker compose up -d
`
	}

	must(os.WriteFile(".github/workflows/main.yaml", []byte(workflowContent), 0o644))

	_ = os.RemoveAll(".git")

	// Initialisation avec la branche 'main' explicitement
	runShell("git init -b main")
	runShell("git config user.name '" + user + "'")
	runShell("git config user.email '" + user + "@localhost'")
	runShell("git config transfer.credentialsInUrl allow")

	remoteURL := fmt.Sprintf("http://%s:%s@localhost:3000/%s/app-repo.git", user, password, user)
	runShell("git remote remove origin || true")
	runShell("git remote add origin " + remoteURL)

	runShell("git add .")
	runShell("git commit -m 'Zero-Touch: Auto-generated pipeline'")
	runShell("git branch -M main")
	runShell("git push -u origin main --force")
	fmt.Println("[+] Pipeline GitOps déployé !")
}

func main() {
	// Étape 1 : Préparer l'exécutable 'mise' (Téléchargement + Extraction)
    misePath := installMise()
    // Étape 2 : Décider s'il faut utiliser le mode avec JSON (Expert) ou mode de l'Auto-détection (Automatique)
    startMode(misePath)
	// étape 3
	isMicro := AutoIsMicroservice()
	forgejoBin, forgejoDir := setupForgejo()
	setupRunner(forgejoBin, forgejoDir)
	user, pass := createAdminAndRepo(forgejoBin, forgejoDir)

	deployGitOps(isMicro, user, pass)

	fmt.Println("\n[🎉] Chaîne complète exécutée avec succès !")
}
