// Package awsproviderpatch provides the `aws-provider-patch` command.
//
// The `aws-provider-patch` command finds all Terraform modules nested in the current code (i.e., in the .terraform/modules
// folder), looks for provider "aws" { ... } blocks in those modules, and overwrites the attributes in those provider
// blocks with the attributes specified in terragrntOptions.
//
// For example, if were running Terragrunt against code that contained a module:
//
//	module "example" {
//	  source = "<URL>"
//	}
//
// When you run 'init', Terraform would download the code for that module into .terraform/modules. This function would
// scan that module code for provider blocks:
//
//	provider "aws" {
//	   region = var.aws_region
//	}
//
// And if AwsProviderPatchOverrides in opts was set to map[string]string{"region": "us-east-1"}, then this
// method would update the module code to:
//
//	provider "aws" {
//	   region = "us-east-1"
//	}
//
// This is a temporary workaround for a Terraform bug (https://github.com/hashicorp/terraform/issues/13018) where
// any dynamic values in nested provider blocks are not handled correctly when you call 'terraform import', so by
// temporarily hard-coding them, we can allow 'import' to work.
package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "aws-provider-patch"

	OverrideAttrFlagName = "override-attr"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	cliRedesignControl := flags.StrictControlsByGroup(opts.StrictControls, CommandName, controls.CLIRedesign)

	return cli.Flags{
		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:        OverrideAttrFlagName,
			EnvVars:     tgPrefix.EnvVars(OverrideAttrFlagName),
			Destination: &opts.AwsProviderPatchOverrides,
			Usage:       "A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix, cliRedesignControl)),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags:  append(run.NewFlags(opts, nil), NewFlags(opts, nil)...).Sort(),
		Action: func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}
