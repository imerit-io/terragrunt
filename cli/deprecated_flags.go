//nolint:unparam
package cli

import (
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

// The following flags are DEPRECATED
const (
	TerragruntIncludeModulePrefixFlagName = "terragrunt-include-module-prefix"
	TerragruntIncludeModulePrefixEnvName  = "TERRAGRUNT_INCLUDE_MODULE_PREFIX"

	TerragruntDisableLogFormattingFlagName = "terragrunt-disable-log-formatting"
	TerragruntDisableLogFormattingEnvName  = "TERRAGRUNT_DISABLE_LOG_FORMATTING"

	TerragruntJsonLogFlagName = "terragrunt-json-log"
	TerragruntJsonLogEnvName  = "TERRAGRUNT_JSON_LOG"
)

// NewDeprecatedFlags creates and returns deprecated flags.
func NewDeprecatedFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		&cli.BoolFlag{
			Name:   TerragruntIncludeModulePrefixFlagName,
			EnvVar: TerragruntIncludeModulePrefixEnvName,
			Usage:  "When this flag is set output from Terraform sub-commands is prefixed with module path.",
			Hidden: true,
			Action: func(ctx *cli.Context) error {
				opts.Logger.Warnf("The %q flag is deprecated. Use the functionality-inverted %q flag instead. By default, Terraform/OpenTofu output is integrated into the Terragrunt log, which prepends additional data, such as timestamps and prefixes, to log entries.", TerragruntIncludeModulePrefixFlagName, commands.TerragruntForwardTFStdoutFlagName)
				return nil
			},
		},
		&cli.BoolFlag{
			Name:   TerragruntDisableLogFormattingFlagName,
			EnvVar: TerragruntDisableLogFormattingEnvName,
			Usage:  "If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.",
			Hidden: true,
			Action: func(ctx *cli.Context) error {
				//opts.LogFormatter = format.NewKeyValueFormat()
				opts.Logger.Warnf("The %q flag is deprecated. Use the %q flag instead.", TerragruntDisableLogFormattingFlagName, commands.TerragruntLogFormatFlagName)
				return nil
			},
		},
		&cli.BoolFlag{
			Name:   TerragruntJsonLogFlagName,
			EnvVar: TerragruntJsonLogEnvName,
			Usage:  "If specified, Terragrunt will output its logs in JSON format.",
			Hidden: true,
			Action: func(ctx *cli.Context) error {
				//opts.LogFormatter = format.NewJSONFormat()
				opts.Logger.Warnf("The %q flag is deprecated. Use the %q flag instead.", TerragruntJsonLogFlagName, commands.TerragruntLogFormatFlagName)
				return nil
			},
		},
	}

	return flags
}
