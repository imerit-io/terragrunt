// Package strict provides utilities used by Terragrunt to support a "strict" mode.
// By default strict mode is disabled, but when Enabled, any breaking changes
// to Terragrunt behavior that is not backwards compatible will result in an error.
//
// Note that any behavior outlined here should be documented in /docs/_docs/04_reference/strict-mode.md
//
// That is how users will know what to expect when they enable strict mode, and how to customize it.
package strict

import (
	"reflect"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/exp/slices"
)

const (
	// DeprecatedFlags is the control that prevents the use of deprecated flag names.
	DeprecatedFlags ControlName = "deprecated-flags"
	// DeprecatedEnvVars is the control that prevents the use of deprecated env vars.
	DeprecatedEnvVars ControlName = "deprecated-env-vars"
	// DeprecatedCommands is the control that prevents the use of deprecated commands.
	DeprecatedCommands ControlName = "deprecated-commands"
	// DeprecatedDefaultCommand is the control that prevents the deprecated default command from being used.
	DeprecatedDefaultCommand ControlName = "deprecated-default-command"
	// SkipDependenciesInputs is the control that prevents reading dependencies inputs and get performance boost.
	SkipDependenciesInputs = "skip-dependencies-inputs"
	// RootTerragruntHCL is the control that prevents usage of a `terragrunt.hcl` file as the root of Terragrunt configurations.
	RootTerragruntHCL ControlName = "root-terragrunt-hcl"
)

const (
	// StatusOngoing is the Status of a control that is ongoing.
	StatusOngoing byte = iota
	// StatusCompleted is the Status of a control that is completed.
	StatusCompleted
)

// ControlName represents a control name.
type ControlName string

// Controls is are multiple of `Control`.
type Controls []*Control

//nolint:lll
func NewControls() Controls {
	return Controls{
		{
			Name:     DeprecatedFlags,
			ErrorFmt: "--%s` flag is no longer supported. Use `--%s` instead.",
			WarnFmt:  "`--%s` flag is deprecated and will be removed in a future version. Use `--%s` instead.",
		},
		{
			Name:     DeprecatedEnvVars,
			ErrorFmt: "`--%s` env var is no longer supported. Use `--%s` instead.",
			WarnFmt:  "`--%s` env var is deprecated and will be removed in a future version. Use `--%s` instead.",
		},
		{
			Name:     DeprecatedCommands,
			ErrorFmt: "`%s` command is no longer supported. Use `%s` instead.",
			WarnFmt:  "`%s` command is deprecated and will be removed in a future version. Use `%s` instead.",
		},
		{
			Name:     DeprecatedDefaultCommand,
			ErrorFmt: "`%[1]s` command is not a valid Terragrunt command. Use `terragrunt run` to explicitly pass commands to OpenTofu/Terraform instead. e.g. `terragrunt run -- %[1]s`",
			WarnFmt:  "`%[1]s` command is deprecated and will be removed in a future version. Use `terragrunt run -- %[1]s` instead.",
		},
		{
			Name:     RootTerragruntHCL,
			ErrorFmt: "Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern, and no longer supported. Use a differently named file like `root.hcl` instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/migrating-from-root-terragrunt-hcl",
			WarnFmt:  "Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern, and no longer recommended. In a future version of Terragrunt, this will result in an error. You are advised to use a differently named file like `root.hcl` instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/migrating-from-root-terragrunt-hcl",
		},
		{
			// TODO: `ErrorFmt` and `WarnFmt` of this control are not displayed anywhere and needs to be reworked.
			Name:     SkipDependenciesInputs,
			ErrorFmt: "The `" + SkipDependenciesInputs + "` option is deprecated. Reading inputs from dependencies has been deprecated and will be removed in a future version of Terragrunt. To continue using inputs from dependencies, forward them as outputs.",
			WarnFmt:  "The `" + SkipDependenciesInputs + "` option is deprecated and will be removed in a future version of Terragrunt. Reading inputs from dependencies has been deprecated. To continue using inputs from dependencies, forward them as outputs.",
		},
	}
}

// Names returns all strict control names.
func (controls Controls) Names() []string {
	names := []string{}

	for _, control := range controls {
		names = append(names, string(control.Name))
	}

	slices.Sort(names)

	return names
}

// FilterByStatus returns controls filtered by the given `status`.
func (controls Controls) FilterByStatus(status byte) Controls {
	var found Controls

	for _, control := range controls {
		if control.Status == status {
			found = append(found, control)
		}
	}

	return found
}

// Find searches and returns the control by the given `name`.
func (controls Controls) Find(name ControlName) *Control {
	for _, control := range controls {
		if control.Name == name {
			return control
		}
	}

	return nil
}

// EnableStrictMode enables the strict mode.
func (controls Controls) EnableStrictMode() {
	for _, control := range controls.FilterByStatus(StatusOngoing) {
		control.Enabled = true
	}
}

// EnableControl validates that the specified control name is valid and enables this control.
func (controls Controls) EnableControl(name string) error {
	if control := controls.Find(ControlName(name)); control != nil {
		control.Enabled = true

		return nil
	}

	return NewInvalidControlNameError(controls.FilterByStatus(StatusOngoing).Names())
}

// NotifyCompletedControls logs the control names that are Enabled and have completed Status.
func (controls Controls) NotifyCompletedControls(logger log.Logger) {
	var completed Controls

	for _, control := range controls.FilterByStatus(StatusCompleted) {
		if control.Enabled {
			completed = append(completed, control)
		}
	}

	if len(completed) == 0 {
		return
	}

	logger.Warnf(NewCompletedControlsError(completed.Names()).Error())
}

// Evaluate returns an error if the control is Enabled otherwise logs the warning message and returns nil.
// If the control is not found, returns nil.
func (controls Controls) Evaluate(logger log.Logger, name ControlName, fmtArgs ...any) error {
	if control := controls.FilterByStatus(StatusOngoing).Find(name); control != nil {
		if err := control.Evaluate(logger, fmtArgs...); err != nil {
			return err
		}
	}

	return nil
}

// Control represents a control that can be Enabled or disabled in strict mode.
// When the control is Enabled, Terragrunt will behave in a way that is not backwards compatible.
type Control struct {
	// Name is the name of the control.
	Name ControlName
	// ErrorFmt is the error that will be returned when the control is Enabled.
	ErrorFmt string
	// WarnFmt is a warning that will be logged when the control is not Enabled.
	WarnFmt string
	// Enabled indicates that the control is Enabled.
	Enabled bool
	// Status of the strict control.
	Status byte
	// triggeredArgs keeps `fmtArgs` that have previously triggered a warning message.
	triggeredArgs [][]any
}

func (control *Control) String() string {
	return string(control.Name)
}

// Evaluate returns an error if the control is Enabled otherwise logs the warning message returns nil.
func (control *Control) Evaluate(logger log.Logger, fmtArgs ...any) error {
	if control.Status == StatusCompleted {
		return nil
	}

	if control.Enabled && control.ErrorFmt != "" {
		return errors.Errorf(control.ErrorFmt, fmtArgs...)
	}

	if control.WarnFmt == "" || logger == nil {
		return nil
	}

	for _, triggeredArgs := range control.triggeredArgs {
		if reflect.DeepEqual(triggeredArgs, fmtArgs) {
			return nil
		}
	}

	control.triggeredArgs = append(control.triggeredArgs, fmtArgs)

	logger.Warnf(control.WarnFmt, fmtArgs...)

	return nil
}
