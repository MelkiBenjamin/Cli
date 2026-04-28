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

func runMise(misePath string, tools []Tool) {
	var args []string
	args = append(args, "install")
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

	cmd := exec.Command(misePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	must(cmd.Run())
}

func hasTool(tools []Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func runShell(command string) {
	fmt.Println("Avant commande:", command)
	cmd := exec.Command("sh", "-lc", `export PATH="$HOME/.local/bin:$PATH" && mise x -- `+command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	must(cmd.Run())
	fmt.Println("Après commande:", command)
}

func runPostCommands(tools []Tool) {
	if hasTool(tools, "docker") {
		runShell("dockerizer .")
		runShell("sed -i '1,5d' Dockerfile")
		runShell("sed -i '1,3d' docker-compose.yml")
        runShell(`grep -qrE "ListenAndServe|http\.Serve|:8080" --include="*.go" . || sed -i -e "/EXPOSE/d" -e "/HEALTHCHECK/,+1d" Dockerfile`)
//		runShell("docker build .")
	}

	if hasTool(tools, "kompose") {
		runShell("cp .env.example .env")
		runShell("kompose convert")
	}

	if hasTool(tools, "helm") {
		runShell("kompose convert -c")
	}

	//if hasTool(tools, "k3s") {
	//	runShell("k3s")
	//}
}

func handleAutoMode(misePath string) {
	fmt.Println("🤖 Aucun Install.json. Lancement du mode automatique (dockerizer)...")
	// 1. Installation forcée de Docker & Dockerizer
	runShell("ls /home")
	runMise(misePath, bundles["docker"])
	// 2. Génération automatique
	runShell("dockerizer .")
	runShell("sed -i '1,5d' Dockerfile")
	runShell("sed -i '1,3d' docker-compose.yml")
    runShell(`grep -qrE "http\.ListenAndServe|http\.Serve|Listen\(" --include="*.go" . || sed -i -e "/EXPOSE/d" -e "/HEALTHCHECK/,+1d" Dockerfile`)
	// 3. Analyse du résultat pour décider si on passe sur K8s
	data, err := os.ReadFile("docker-compose.yml")
	if err == nil {
		content := string(data)
		// Si le compose contient plusieurs services, on considère que c'est du microservice
		if strings.Count(content, "image:") > 1 {
			fmt.Println("🏢 Architecture multiple détectée -> Migration vers Kubernetes/Helm")
			// Installation de Kompose (via bundle kubectl) et Helm
			k8sTools := append(bundles["kubectl"], bundles["helm"]...)
			runMise(misePath, k8sTools)
			// Conversion
			runShell("kompose convert -c")
		} else {
			fmt.Println("📦 Monolithe détecté -> On reste sur Docker Compose.")
		}
	}
}

func main() {
	dir := localBin()
	misePath := extractMiseFromURL(latestURL, dir)
	fmt.Println("mise installé dans", misePath)

	if _, err := os.Stat("Install.json"); err == nil {
		// --- MODE 1 : EXPERT ---
		tools := readTools("Install.json")
		expanded := expand(tools)
		runMise(misePath, expanded)
		runPostCommands(expanded)
	} else {
		// --- MODE 2 : AUTOMATIQUE ---
		handleAutoMode(misePath)
	}
}
