package capability

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/modulebuild"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/everFinance/goether"
	hymxSchema "github.com/hymatrix/hymx/schema"
	hymxUtils "github.com/hymatrix/hymx/utils"
	"github.com/permadao/goar"
	arSchema "github.com/permadao/goar/schema"
)

// ExportOptions configures image rebuild and module signing for Export.
type ExportOptions struct {
	AgentBinPath string
	BuildTag     string
	SignerKey    string
}

// ExportResult contains the signed module JSON and the collected public preview.
type ExportResult struct {
	ModuleBytes []byte
	Collection  Collection
}

// Export collects public.zip, rebuilds the image from profile.toml, and returns
// a signed V2 module JSON containing image.tar.gz + profile.toml + public.zip.
func Export(ctx context.Context, home string, opts ExportOptions) (ExportResult, error) {
	profilePath := filepath.Join(home, "profile.toml")
	profileTOML, err := os.ReadFile(profilePath)
	if err != nil {
		return ExportResult{}, err
	}
	profile, err := modulebuild.ParseProfile(profileTOML)
	if err != nil {
		return ExportResult{}, err
	}
	// Fail fast on malformed public entries. compilePublicPatterns warn-and-skips
	// bad entries — a fail-closed property the import allowlist depends on — but
	// for a mutating export that silently drops the author's declared files (e.g.
	// entries still in the pre-"~/" legacy format), producing a "successful"
	// module with an empty public.zip. Preview/dry_run stays lenient so authors
	// can iterate; only the artifact-producing path is strict.
	if _, rejected := compilePublicPatterns(profile.Vmdocker.Public); len(rejected) > 0 {
		return ExportResult{}, fmt.Errorf("profile has %d invalid public entry(ies); fix them before export: %s",
			len(rejected), strings.Join(rejected, "; "))
	}
	col, err := CollectPublic(home, profile.Vmdocker.Public)
	if err != nil {
		return ExportResult{}, err
	}
	publicZip, err := BuildPublicZip(home, profile.Vmdocker.Public)
	if err != nil {
		return ExportResult{}, err
	}
	artifact, err := modulebuild.BuildModuleArtifact(ctx, modulebuild.BuildOptions{
		ProfileTOML:  profileTOML,
		ProfileDir:   home,
		AgentBinPath: opts.AgentBinPath,
		BuildTag:     opts.BuildTag,
		PublicZip:    publicZip,
	})
	if err != nil {
		return ExportResult{}, err
	}
	moduleBytes, err := SignModuleArtifact(artifact, opts.SignerKey)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{ModuleBytes: moduleBytes, Collection: col}, nil
}

// SignModuleArtifact wraps a modulebuild artifact in a signed BundleItem JSON.
func SignModuleArtifact(artifact modulebuild.ModuleArtifact, signerKey string) ([]byte, error) {
	if signerKey == "" {
		key, err := crypto.GenerateKey()
		if err != nil {
			return nil, err
		}
		signerKey = hex.EncodeToString(crypto.FromECDSA(key))
	}
	signer, err := goether.NewSigner(signerKey)
	if err != nil {
		return nil, err
	}
	bundler, err := goar.NewBundler(signer)
	if err != nil {
		return nil, err
	}
	tags, err := hymxUtils.ModuleToTags(hymxSchema.Module{
		Base:         hymxSchema.DefaultBaseModule,
		ModuleFormat: modulebuild.ModuleFormat,
		Tags:         artifact.Tags,
	})
	if err != nil {
		return nil, err
	}
	item, err := bundler.CreateAndSignItem(artifact.ModuleBytes, "", "", tags)
	if err != nil {
		return nil, err
	}
	item.Tags = normalizeSignedTags(item.Tags, tags)
	b, err := json.Marshal(item)
	if err != nil {
		return nil, fmt.Errorf("marshal module bundle item: %w", err)
	}
	return b, nil
}

func normalizeSignedTags(itemTags, plainTags []arSchema.Tag) []arSchema.Tag {
	if tagValue(itemTags, "Module-Format") != "" {
		return itemTags
	}
	return plainTags
}
