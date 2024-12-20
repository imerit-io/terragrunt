// Package renderjson provides the command to render the final terragrunt config, with all variables, includes, and functions resolved, as json.
package renderjson

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "render-json"

	OutFlagName                     = "out"
	WithMetadataFlagName            = "with-metadata"
	DisableDependentModulesFlagName = "disable-dependent-modules"

	TerragruntJSONOutFlagName                 = flags.DeprecatedFlagNamePrefix + "json-out"
	TerragruntDisableDependentModulesFlagName = flags.DeprecatedFlagNamePrefix + "json-disable-dependent-modules"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		flags.GenericWithDeprecatedFlag(opts, &cli.GenericFlag[string]{
			Name:        OutFlagName,
			EnvVars:     flags.EnvVars(OutFlagName),
			Destination: &opts.JSONOut,
			Usage:       "The file path that terragrunt should use when rendering the terragrunt.hcl config as json.",
		}, TerragruntJSONOutFlagName),
		&cli.BoolFlag{
			Name:        WithMetadataFlagName,
			EnvVars:     flags.EnvVars(WithMetadataFlagName),
			Destination: &opts.RenderJSONWithMetadata,
			Usage:       "Add metadata to the rendered JSON file.",
		},
		flags.BoolWithDeprecatedFlag(opts, &cli.BoolFlag{
			Name:        DisableDependentModulesFlagName,
			EnvVars:     flags.EnvVars(DisableDependentModulesFlagName),
			Destination: &opts.JSONDisableDependentModules,
			Usage:       "Disable identification of dependent modules rendering json config.",
		}, TerragruntDisableDependentModulesFlagName),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, as json.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:       append(flags.NewCommonFlags(opts), NewFlags(opts)...).Sort(),
		Action:      func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}
