package codegen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/hashicorp/hcl/v2/hclwrite"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	// A comment that is added to the top of the generated file to indicate that this file was generated by Terragrunt.
	// We use a hardcoded random string at the end to make the string further unique.
	TerragruntGeneratedSignature = "Generated by Terragrunt. Sig: nIlQXj57tbuaRZEa"

	// The default prefix to use for comments in the generated file
	DefaultCommentPrefix = "# "
)

// GenerateConfigExists is an enum to represent valid values for if_exists.
type GenerateConfigExists int

const (
	ExistsError GenerateConfigExists = iota
	ExistsSkip
	ExistsOverwrite
	ExistsOverwriteTerragrunt
	ExistsUnknown
)

// GenerateConfigDisabled is an enum to represent valid values for if_disabled.
type GenerateConfigDisabled int

const (
	DisabledSkip GenerateConfigDisabled = iota
	DisabledRemove
	DisabledRemoveTerragrunt
	DisabledUnknown
)

const (
	ExistsErrorStr               = "error"
	ExistsSkipStr                = "skip"
	ExistsOverwriteStr           = "overwrite"
	ExistsOverwriteTerragruntStr = "overwrite_terragrunt"

	DisabledSkipStr             = "skip"
	DisabledRemoveStr           = "remove"
	DisabledRemoveTerragruntStr = "remove_terragrunt"

	assumeRoleConfigKey = "assume_role"

	encryptionBlockName = "encryption"

	EncryptionKeyProviderKey = "key_provider"
	encryptionResourceName   = "default"

	encryptionMethodKey     = "method"
	encryptionDefaultMethod = "aes_gcm"

	encryptionKeysAttributeName = "keys"

	encryptionStateBlockName = "state"
	encryptionPlanBlockName  = "plan"
)

// GenerateConfig is configuration for generating code
type GenerateConfig struct {
	Path             string `cty:"path"`
	IfExists         GenerateConfigExists
	IfExistsStr      string `cty:"if_exists"`
	IfDisabled       GenerateConfigDisabled
	IfDisabledStr    string `cty:"if_disabled"`
	CommentPrefix    string `cty:"comment_prefix"`
	Contents         string `cty:"contents"`
	DisableSignature bool   `cty:"disable_signature"`
	Disable          bool   `cty:"disable"`
	HclFmt           *bool  `cty:"hcl_fmt"`
}

// WriteToFile will generate a new file at the given target path with the given contents. If a file already exists at
// the target path, the behavior depends on the value of IfExists:
// - if ExistsError, return an error.
// - if ExistsSkip, do nothing and return
// - if ExistsOverwrite, overwrite the existing file
func WriteToFile(terragruntOptions *options.TerragruntOptions, basePath string, config GenerateConfig) error {
	// Figure out thee target path to generate the code in. If relative, merge with basePath.
	var targetPath string
	if filepath.IsAbs(config.Path) {
		targetPath = config.Path
	} else {
		targetPath = filepath.Join(basePath, config.Path)
	}

	targetFileExists := util.FileExists(targetPath)

	// If this GenerateConfig is disabled then skip further processing.
	if config.Disable {
		terragruntOptions.Logger.Debugf("Skipping generating file at %s because it is disabled", config.Path)

		if targetFileExists {
			if shouldRemove, err := shouldRemoveWithFileExists(terragruntOptions, targetPath, config.IfDisabled); err != nil {
				return err
			} else if shouldRemove {
				if err := os.Remove(targetPath); err != nil {
					return errors.New(err)
				}
			}
		}

		return nil
	}

	if targetFileExists {
		shouldContinue, err := shouldContinueWithFileExists(terragruntOptions, targetPath, config.IfExists)
		if err != nil || !shouldContinue {
			return err
		}
	}

	// Add the signature as a prefix to the file, unless it is disabled.
	prefix := ""
	if !config.DisableSignature {
		prefix = fmt.Sprintf("%s%s\n", config.CommentPrefix, TerragruntGeneratedSignature)
	}

	fmtGeneratedCode := false

	if config.HclFmt == nil {
		var fmtExt = map[string]struct{}{
			".hcl":  {},
			".tf":   {},
			".tofu": {},
		}

		ext := filepath.Ext(config.Path)
		if _, ok := fmtExt[ext]; ok {
			fmtGeneratedCode = true
		}
	} else {
		fmtGeneratedCode = *config.HclFmt
	}

	contentsToWrite := []byte(fmt.Sprintf("%s%s", prefix, config.Contents))
	if fmtGeneratedCode {
		contentsToWrite = hclwrite.Format(contentsToWrite)
	}

	const ownerWriteGlobalReadPerms = 0644
	if err := os.WriteFile(targetPath, contentsToWrite, ownerWriteGlobalReadPerms); err != nil {
		return errors.New(err)
	}

	terragruntOptions.Logger.Debugf("Generated file %s.", targetPath)

	return nil
}

// Whether or not file generation should continue if the file path already exists. The answer depends on the
// ifExists configuration.
func shouldContinueWithFileExists(terragruntOptions *options.TerragruntOptions, path string, ifExists GenerateConfigExists) (bool, error) {
	// TODO: Make exhaustive
	switch ifExists { //nolint:exhaustive
	case ExistsError:
		return false, errors.New(GenerateFileExistsError{path: path})
	case ExistsSkip:
		// Do nothing since file exists and skip was configured
		terragruntOptions.Logger.Debugf("The file path %s already exists and if_exists for code generation set to \"skip\". Will not regenerate file.", path)

		return false, nil
	case ExistsOverwrite:
		// We will continue to proceed to generate file, but log a message to indicate that we detected the file
		// exists.
		terragruntOptions.Logger.Debugf("The file path %s already exists and if_exists for code generation set to \"overwrite\". Regenerating file.", path)

		return true, nil
	case ExistsOverwriteTerragrunt:
		// If file was not generated, error out because overwrite_terragrunt if_exists setting only handles if the
		// existing file was generated by terragrunt.
		wasGenerated, err := fileWasGeneratedByTerragrunt(path)
		if err != nil {
			return false, err
		}

		if !wasGenerated {
			terragruntOptions.Logger.Errorf("ERROR: The file path %s already exists and was not generated by terragrunt.", path)

			return false, errors.New(GenerateFileExistsError{path: path})
		}

		// Since file was generated by terragrunt, continue.
		terragruntOptions.Logger.Debugf("The file path %s already exists, but was a previously generated file by terragrunt. Since if_exists for code generation is set to \"overwrite_terragrunt\", regenerating file.", path)

		return true, nil
	default:
		// This shouldn't happen, but we add this case anyway for defensive coding.
		return false, errors.New(UnknownGenerateIfExistsVal{""})
	}
}

// shouldRemoveWithFileExists returns true if the already existing file should be removed.
func shouldRemoveWithFileExists(terragruntOptions *options.TerragruntOptions, path string, ifDisable GenerateConfigDisabled) (bool, error) {
	// TODO: Make exhaustive
	switch ifDisable { //nolint:exhaustive
	case DisabledSkip:
		// Do nothing since skip was configured.
		terragruntOptions.Logger.Debugf("The file path %s already exists and if_disabled for code generation set to \"skip\", will not remove file.", path)
		return false, nil
	case DisabledRemove:
		// The file exists and will be removed.
		terragruntOptions.Logger.Debugf("The file path %s already exists and if_disabled for code generation set to \"remove\", removing file.", path)
		return true, nil
	case DisabledRemoveTerragrunt:
		// If file was not generated, error out because remove_terragrunt if_disabled setting only handles if the existing file was generated by terragrunt.
		wasGenerated, err := fileWasGeneratedByTerragrunt(path)
		if err != nil {
			return false, err
		}

		if !wasGenerated {
			terragruntOptions.Logger.Errorf("ERROR: The file path %s already exists and was not generated by terragrunt.", path)

			return false, errors.New(GenerateFileRemoveError{path: path})
		}

		// Since file was generated by terragrunt, removing.
		terragruntOptions.Logger.Debugf("The file path %s already exists, but was a previously generated file by terragrunt. Since if_disabled for code generation is set to \"remove_terragrunt\", removing file.", path)

		return true, nil
	default:
		// This shouldn't happen, but we add this case anyway for defensive coding.
		return false, errors.New(UnknownGenerateIfDisabledVal{""})
	}
}

// Check if the file was generated by terragrunt by checking if the first line of the file has the signature. Since the
// generated string will be prefixed with the configured comment prefix, the check needs to see if the first line ends
// with the signature string.
func fileWasGeneratedByTerragrunt(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, errors.New(err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)

	firstLine, err := reader.ReadString('\n')
	if err != nil {
		return false, errors.New(err)
	}

	return strings.HasSuffix(strings.TrimSpace(firstLine), TerragruntGeneratedSignature), nil
}

const (
	terraformBlock = "terraform"
	backendBlock   = "backend"
)

// RemoteStateConfigToTerraformCode converts the arbitrary map that represents a remote state config into HCL code to configure that remote state.
func RemoteStateConfigToTerraformCode(backend string, config map[string]any, encryption map[string]interface{}) ([]byte, error) {
	f := hclwrite.NewEmptyFile()
	terraformBlock := f.Body().AppendNewBlock(terraformBlock, nil).Body()
	backendBlock := terraformBlock.AppendNewBlock(backendBlock, []string{backend})
	backendBlockBody := backendBlock.Body()

	var backendKeys = make([]string, 0, len(config))

	for key := range config {
		backendKeys = append(backendKeys, key)
	}

	sort.Strings(backendKeys)

	for _, key := range backendKeys {
		// Since we don't have the cty type information for the config and since config can be arbitrary, we cheat by using
		// json as an intermediate representation.
		//
		// handle assume role config key in a different way since it is a single line HCL object
		if key == assumeRoleConfigKey {
			assumeRoleValue, isAssumeRole := config[assumeRoleConfigKey].(string)
			if !isAssumeRole {
				continue
			}
			// extract assume role hcl values to be rendered in HCL
			var assumeRoleMap map[string]string
			// split single line hcl to default multiline file
			hclValue := strings.TrimSuffix(assumeRoleValue, "}")
			hclValue = strings.TrimPrefix(hclValue, "{")
			hclValue = strings.ReplaceAll(hclValue, ",", "\n")
			// basic decode of hcl to a map
			err := hclsimple.Decode("s3_assume_role.hcl", []byte(hclValue), nil, &assumeRoleMap)
			if err != nil {
				return nil, errors.New(err)
			}
			// write assume role map as HCL object
			ctyVal, err := convertValue(assumeRoleMap)
			if err != nil {
				return nil, errors.New(err)
			}

			backendBlockBody.SetAttributeValue(key, ctyVal.Value)

			continue
		}

		ctyVal, err := convertValue(config[key])
		if err != nil {
			return nil, errors.New(err)
		}

		backendBlockBody.SetAttributeValue(key, ctyVal.Value)
	}

	// encryption can be empty
	if len(encryption) == 0 {
		return f.Bytes(), nil
	}

	// extract key_provider first to create key_provider block
	keyProvider, found := encryption[EncryptionKeyProviderKey].(string)
	if !found {
		return nil, errors.New(EncryptionKeyProviderKey + " is mandatory but not found in the encryption map")
	}

	keyProviderTraversal := hcl.Traversal{
		hcl.TraverseRoot{Name: EncryptionKeyProviderKey},
		hcl.TraverseAttr{Name: keyProvider},
		hcl.TraverseAttr{Name: encryptionResourceName},
	}

	methodTraversal := hcl.Traversal{
		hcl.TraverseRoot{Name: encryptionMethodKey},
		hcl.TraverseAttr{Name: encryptionDefaultMethod},
		hcl.TraverseAttr{Name: encryptionResourceName},
	}

	// encryption block
	encryptionBlock := terraformBlock.AppendNewBlock(encryptionBlockName, nil)
	encryptionBlockBody := encryptionBlock.Body()

	// Append key_provider block
	keyProviderBlockBody := encryptionBlockBody.AppendNewBlock(EncryptionKeyProviderKey, []string{keyProvider, encryptionResourceName}).Body()

	// Append method block
	methodBlock := encryptionBlockBody.AppendNewBlock(encryptionMethodKey, []string{encryptionDefaultMethod, encryptionResourceName}).Body()
	methodBlock.SetAttributeTraversal(encryptionKeysAttributeName, keyProviderTraversal)

	// Append state block
	stateBlock := encryptionBlockBody.AppendNewBlock(encryptionStateBlockName, nil).Body()
	stateBlock.SetAttributeTraversal(encryptionMethodKey, methodTraversal)

	// Append plan block
	planBlock := encryptionBlockBody.AppendNewBlock(encryptionPlanBlockName, nil).Body()
	planBlock.SetAttributeTraversal(encryptionMethodKey, methodTraversal)

	var encryptionKeys = make([]string, 0, len(encryption))

	for key := range encryption {
		encryptionKeys = append(encryptionKeys, key)
	}

	sort.Strings(encryptionKeys)

	// Fill key_provider block with ordered attributes
	for _, key := range encryptionKeys {
		if key == EncryptionKeyProviderKey {
			continue
		}

		value, ok := encryption[key]
		if !ok {
			continue
		}

		// Skip basic types with zero values
		if value == "" || value == 0 {
			continue
		}

		ctyVal, err := convertValue(value)
		if err != nil {
			return nil, errors.New(err)
		}

		if keyProviderBlockBody != nil {
			keyProviderBlockBody.SetAttributeValue(key, ctyVal.Value)
		}
	}

	return f.Bytes(), nil
}

func convertValue(v interface{}) (ctyjson.SimpleJSONValue, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return ctyjson.SimpleJSONValue{}, errors.New(err)
	}

	var ctyVal ctyjson.SimpleJSONValue
	if err := ctyVal.UnmarshalJSON(jsonBytes); err != nil {
		return ctyjson.SimpleJSONValue{}, errors.New(err)
	}

	return ctyVal, nil
}

// GenerateConfigExistsFromString converts a string representation of if_exists into the enum, returning an error if it
// is not set to one of the known values.
func GenerateConfigExistsFromString(val string) (GenerateConfigExists, error) {
	switch val {
	case ExistsErrorStr:
		return ExistsError, nil
	case ExistsSkipStr:
		return ExistsSkip, nil
	case ExistsOverwriteStr:
		return ExistsOverwrite, nil
	case ExistsOverwriteTerragruntStr:
		return ExistsOverwriteTerragrunt, nil
	}

	return ExistsUnknown, errors.New(UnknownGenerateIfExistsVal{val: val})
}

// GenerateConfigDisabledFromString converts a string representation of if_disabled into the enum, returning an error if it is not set to one of the known values.
func GenerateConfigDisabledFromString(val string) (GenerateConfigDisabled, error) {
	switch val {
	case DisabledSkipStr:
		return DisabledSkip, nil
	case DisabledRemoveStr:
		return DisabledRemove, nil
	case DisabledRemoveTerragruntStr:
		return DisabledRemoveTerragrunt, nil
	}

	return DisabledUnknown, errors.New(UnknownGenerateIfDisabledVal{val: val})
}
