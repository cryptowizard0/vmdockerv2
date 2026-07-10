package capability

import (
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

// ExportOptions configures module packing and signing for Export.
//
// Export reuses the agent's existing image (the one it was spawned from) rather
// than rebuilding from profile.toml: the image-build inputs (bin/, start.sh)
// live baked in the image at /usr/local/*, not in the runtime workspace, so a
// rebuild from the live workspace is impossible. The caller reads image.tar.gz
// out of the running process's module and passes it here.
type ExportOptions struct {
	ImageArchive []byte // the running agent's image.tar.gz, reused as-is
	ImageName    string // Image-Name tag carried onto the new module
	ImageID      string // Image-ID tag carried onto the new module
	SignerKey    string // hex signer key; empty -> ephemeral key
}

// ExportResult contains the signed module JSON and the collected public preview.
type ExportResult struct {
	ModuleBytes []byte
	Collection  Collection
}

// Export freshly collects public.zip from the live workspace, packs it together
// with the reused image.tar.gz and the current profile.toml, and returns a
// signed V2 module JSON. The program (image, incl. bin/ and start.sh) is carried
// over unchanged; only the public state is re-captured at export time.
func Export(home string, opts ExportOptions) (ExportResult, error) {
	profilePath := filepath.Join(home, "profile.toml")
	profileTOML, err := os.ReadFile(profilePath)
	if err != nil {
		return ExportResult{}, err
	}
	profile, err := modulebuild.ParseProfile(profileTOML)
	if err != nil {
		return ExportResult{}, err
	}
	if len(opts.ImageArchive) == 0 {
		return ExportResult{}, fmt.Errorf("export requires the running agent's image (ImageArchive); export does not rebuild from profile")
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
	artifact, err := modulebuild.PackModule(modulebuild.PackInput{
		ImageArchive:    opts.ImageArchive,
		ImageName:       opts.ImageName,
		ImageID:         opts.ImageID,
		ProfileTOML:     profileTOML,
		PublicZip:       publicZip,
		Public:          profile.Vmdocker.Public,
		IncludeImageSHA: true,
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
