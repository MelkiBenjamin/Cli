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

func main() {
	// Étape 1 : Préparer l'exécutable 'mise' (Téléchargement + Extraction)
    misePath := installMise()
    // Étape 2 : Décider s'il faut utiliser le mode avec JSON (Expert) ou mode de l'Auto-détection (Automatique)
    startMode(misePath)
}
