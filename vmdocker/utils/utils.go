package utils

import (
	"errors"
	"fmt"

	"github.com/cryptowizard0/vmdockerv2/vmdocker/runtimemanager/schema"
	hySchema "github.com/hymatrix/hymx/schema"
	"github.com/hymatrix/hymx/utils"
	goarSchema "github.com/permadao/goar/schema"
)

// todo: this function for test env, should remove when real data is ok
func BuildProcessTags(process hySchema.Process, nodeAddr string, tags []goarSchema.Tag) (hySchema.Process, error) {
	processTags, err := utils.ProcessToTags(process)
	if err != nil {
		return process, err
	}

	processTags = append(processTags, []goarSchema.Tag{
		{
			Name:  "Authority",
			Value: nodeAddr,
		},
		{
			Name:  "App-Name",
			Value: "hymatrix",
		},
		{
			Name:  "Name",
			Value: utils.GetTagsValueByDefault("Name", tags, "default"),
		},
		{
			Name:  "Content-Type",
			Value: "text/plain",
		},
		{
			Name:  "Reference",
			Value: utils.GetTagsValueByDefault("Reference", tags, "0"),
		},
	}...)

	fromProcess := utils.GetTagsValueByDefault("From-Process", tags, "")
	if fromProcess != "" {
		processTags = append(processTags, goarSchema.Tag{
			Name:  "From-Process",
			Value: fromProcess,
		})
	}
	process.Tags = processTags
	return process, nil
}

func RuntimeSpecFromTags(moduleFormat string, tags []goarSchema.Tag) (schema.RuntimeSpec, error) {
	switch moduleFormat {
	case schema.ModuleFormat:
	default:
		return schema.RuntimeSpec{}, fmt.Errorf("unsupported module format %q", moduleFormat)
	}

	imageInfo, err := imageInfoFromTags(tags)
	if err != nil {
		return schema.RuntimeSpec{}, err
	}

	spec := schema.RuntimeSpec{
		Backend: "",
		Image:   imageInfo,
		Sandbox: schema.SandboxSpec{
			Agent:     utils.GetTagsValueByDefault(schema.SandboxAgentTag, tags, "shell"),
			Workspace: "",
			Network:   utils.GetTagsValueByDefault(schema.SandboxNetworkTag, tags, ""),
			Name:      utils.GetTagsValueByDefault(schema.SandboxNameTag, tags, ""),
		},
	}

	return spec, nil
}

func RuntimeSpecFromModuleAndSpawnTags(moduleFormat string, moduleTags, spawnTags []goarSchema.Tag) (schema.RuntimeSpec, error) {
	spec, err := RuntimeSpecFromTags(moduleFormat, moduleTags)
	if err != nil {
		return schema.RuntimeSpec{}, err
	}

	backend := runtimeBackendFromTags(spawnTags)
	if backend != "" {
		switch backend {
		case schema.RuntimeBackendDocker, schema.RuntimeBackendSandbox:
			spec.Backend = backend
		default:
			return schema.RuntimeSpec{}, errors.New("unsupported runtime backend: " + backend)
		}
	}

	return spec, nil
}

func runtimeBackendFromTags(tags []goarSchema.Tag) string {
	for i := len(tags) - 1; i >= 0; i-- {
		if tags[i].Name == schema.RuntimeBackendTag {
			return tags[i].Value
		}
	}
	return ""
}

func imageInfoFromTags(tags []goarSchema.Tag) (schema.ImageInfo, error) {
	if buildType := utils.GetTagsValueByDefault("Build-Type", tags, ""); buildType != "" {
		return schema.ImageInfo{}, errors.New("Build-Type modules are no longer supported")
	}

	imageName := utils.GetTagsValueByDefault(schema.ImageNameTag, tags, "")
	if imageName == "" {
		return schema.ImageInfo{}, errors.New(schema.ImageNameTag + " is empty")
	}

	imageID := utils.GetTagsValueByDefault(schema.ImageIDTag, tags, "")
	if imageID == "" {
		return schema.ImageInfo{}, errors.New(schema.ImageIDTag + " is empty")
	}

	source := utils.GetTagsValueByDefault(schema.ImageSourceTag, tags, "")
	if source != schema.ImageSourceModuleData {
		return schema.ImageInfo{}, fmt.Errorf("%s must be %s", schema.ImageSourceTag, schema.ImageSourceModuleData)
	}

	archiveFormat := utils.GetTagsValueByDefault(schema.ImageArchiveTag, tags, "")
	if archiveFormat != schema.ImageArchiveDockerSaveGZ && archiveFormat != schema.ImageArchiveContainerTarGZ {
		return schema.ImageInfo{}, fmt.Errorf("%s must be %s or %s", schema.ImageArchiveTag, schema.ImageArchiveDockerSaveGZ, schema.ImageArchiveContainerTarGZ)
	}

	return schema.ImageInfo{
		Name:          imageName,
		SHA:           imageID,
		Source:        source,
		ArchiveFormat: archiveFormat,
	}, nil
}

// CheckModuleFormat validates the module configuration for the selected runtime backend.
func CheckModuleFormat(moduleFormat string, tags []goarSchema.Tag) error {
	_, err := RuntimeSpecFromTags(moduleFormat, tags)
	if err != nil {
		return err
	}

	return nil
}
