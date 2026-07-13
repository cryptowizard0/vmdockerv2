package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/capability"
	"github.com/cryptowizard0/vmdockerv2/vmdocker/modulebuild"
	"github.com/everFinance/goether"
	hymxSchema "github.com/hymatrix/hymx/schema"
	"github.com/hymatrix/hymx/sdk"
	"github.com/permadao/goar"
)

func main() {
	loadEnv()

	profilePath := flag.String("profile", "profile.toml", "path to profile.toml")
	agentBin := flag.String("agent-bin", os.Getenv("VMDOCKER_AGENT_BIN"), "path to platform adapter binary")
	flag.Parse()

	// An empty --agent-bin (e.g. `--agent-bin "$UNSET_VAR"`) falls back to the env / .env value.
	if strings.TrimSpace(*agentBin) == "" {
		*agentBin = os.Getenv("VMDOCKER_AGENT_BIN")
	}

	profileTOML, err := os.ReadFile(*profilePath)
	if err != nil {
		fatal("read profile: %v", err)
	}
	if *agentBin == "" {
		fatal("-agent-bin (platform B2 artifact) is required; set VMDOCKER_AGENT_BIN in .env")
	}

	// Collect the profile's [vmdocker].public files into public.zip so the built
	// module ships its initial public state (skills, persona, ...). Without this,
	// the allowlist is inert at build time and public content only ever appears
	// via a later Export — the profile dir's files would be silently dropped.
	profileDir := filepath.Dir(*profilePath)
	publicZip, col, err := collectProfilePublicZip(profileDir, profileTOML)
	if err != nil {
		fatal("collect public: %v", err)
	}
	fmt.Printf("[module] collected %d public file(s), %d bytes\n", len(col.Entries), col.TotalBytes)
	for _, w := range col.Warnings {
		fmt.Printf("[module] public warning: %s\n", w)
	}

	fmt.Println("[module] building module artifact from profile")
	artifact, err := modulebuild.BuildModuleArtifact(context.Background(), modulebuild.BuildOptions{
		ProfileTOML:  profileTOML,
		ProfileDir:   profileDir,
		AgentBinPath: *agentBin,
		PublicZip:    publicZip,
	})
	if err != nil {
		fatal("build module artifact: %v", err)
	}
	fmt.Printf("[module] artifact ready: tags=%d payload=%d bytes\n", len(artifact.Tags), len(artifact.ModuleBytes))

	client, err := newSDK()
	if err != nil {
		fatal("init sdk: %v", err)
	}
	itemID, err := client.SaveModule(artifact.ModuleBytes, hymxSchema.Module{
		Base:         hymxSchema.DefaultBaseModule,
		ModuleFormat: modulebuild.ModuleFormat,
		Tags:         artifact.Tags,
	})
	if err != nil {
		fatal("save module: %v", err)
	}
	fmt.Printf("[module] saved module %s -> mod-%s.json\n", itemID, itemID)
}

// collectProfilePublicZip collects the profile's [vmdocker].public files from
// the profile directory into a public.zip (and a preview), mirroring what
// Export does from a live workspace. This is what puts the author's initial
// skills/persona/... into the built module so spawn seeds them.
func collectProfilePublicZip(profileDir string, profileTOML []byte) ([]byte, capability.Collection, error) {
	profile, err := modulebuild.ParseProfile(profileTOML)
	if err != nil {
		return nil, capability.Collection{}, err
	}
	col, err := capability.CollectPublic(profileDir, profile.Vmdocker.Public)
	if err != nil {
		return nil, capability.Collection{}, err
	}
	publicZip, err := capability.BuildPublicZip(profileDir, profile.Vmdocker.Public)
	if err != nil {
		return nil, capability.Collection{}, err
	}
	return publicZip, col, nil
}

func newSDK() (*sdk.SDK, error) {
	url := getEnvWith("VMDOCKER_URL", "http://127.0.0.1:8080")
	prvKey := os.Getenv("VMDOCKER_PRIVATE_KEY")
	if prvKey == "" {
		return nil, fmt.Errorf("VMDOCKER_PRIVATE_KEY is required to sign the module")
	}
	signer, err := goether.NewSigner(prvKey)
	if err != nil {
		return nil, err
	}
	bundler, err := goar.NewBundler(signer)
	if err != nil {
		return nil, err
	}
	return sdk.NewFromBundler(url, bundler), nil
}

func getEnvWith(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// loadEnv loads KEY=value pairs from a .env file (VMDOCKER_ENV_FILE, else ./.env)
// so VMDOCKER_AGENT_BIN / VMDOCKER_URL / VMDOCKER_PRIVATE_KEY can be set once
// instead of exported on every run. Real environment variables take precedence.
func loadEnv() {
	path := os.Getenv("VMDOCKER_ENV_FILE")
	if path == "" {
		path = ".env"
	}
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" || value == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
