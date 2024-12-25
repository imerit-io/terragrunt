// Package cli configures the Terragrunt CLI app and its commands.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/engine"
	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/terraform"
	"golang.org/x/sync/errgroup"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/gruntwork-io/terragrunt/cli/commands/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclvalidate"
	"github.com/gruntwork-io/terragrunt/cli/flags"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"

	"github.com/gruntwork-io/terragrunt/shell"

	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
	hashicorpversion "github.com/hashicorp/go-version"

	"github.com/gruntwork-io/go-commons/env"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	"github.com/gruntwork-io/terragrunt/cli/commands/catalog"
	execCmd "github.com/gruntwork-io/terragrunt/cli/commands/exec"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	outputmodulegroups "github.com/gruntwork-io/terragrunt/cli/commands/output-module-groups"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	runCmd "github.com/gruntwork-io/terragrunt/cli/commands/run"
	runall "github.com/gruntwork-io/terragrunt/cli/commands/run-all"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	validateinputs "github.com/gruntwork-io/terragrunt/cli/commands/validate-inputs"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

func init() {
	cli.AppVersionTemplate = AppVersionTemplate
	cli.AppHelpTemplate = AppHelpTemplate
	cli.CommandHelpTemplate = CommandHelpTemplate
}

type App struct {
	*cli.App
	opts *options.TerragruntOptions
}

// NewApp creates the Terragrunt CLI App.
func NewApp(opts *options.TerragruntOptions) *App {
	app := cli.NewApp()
	app.Name = "terragrunt"
	app.Usage = "Terragrunt is a flexible orchestration tool that allows Infrastructure as Code written in OpenTofu/Terraform to scale. For documentation, see https://terragrunt.gruntwork.io/."
	app.Author = "Gruntwork <www.gruntwork.io>"
	app.Version = version.GetVersion()
	app.Writer = opts.Writer
	app.ErrWriter = opts.ErrWriter

	app.Flags = flags.NewGlobalFlags(opts)

	app.Commands = append(
		DeprecatedCommands(opts),
		TerragruntCommands(opts)...).WrapAction(WrapWithTelemetry(opts))

	app.Before = beforeAction(opts)
	app.DefaultCommand = runCmd.NewCommand(opts).WrapAction(WrapWithTelemetry(opts)) // by default, if no terragrunt command is specified, run the Terraform command
	app.OsExiter = OSExiter
	app.ExitErrHandler = ExitErrHandler

	return &App{app, opts}
}

func (app *App) Run(args []string) error {
	return app.RunContext(context.Background(), args)
}

func (app *App) registerGracefullyShutdown(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)

	signal.NotifierWithContext(ctx, func(sig os.Signal) {
		// Carriage return helps prevent "^C" from being printed
		fmt.Fprint(app.Writer, "\r") //nolint:errcheck
		app.opts.Logger.Infof("%s signal received. Gracefully shutting down...", cases.Title(language.English).String(sig.String()))

		cancel(signal.NewContextCanceledError(sig))
	}, signal.InterruptSignals...)

	return ctx
}

func (app *App) RunContext(ctx context.Context, args []string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ctx = app.registerGracefullyShutdown(ctx)

	err := telemetry.InitTelemetry(ctx, &telemetry.TelemetryOptions{
		Vars:       env.Parse(os.Environ()),
		AppName:    app.Name,
		AppVersion: app.Version,
		Writer:     app.Writer,
		ErrWriter:  app.ErrWriter,
	})
	if err != nil {
		return err
	}
	defer func(ctx context.Context) {
		if err := telemetry.ShutdownTelemetry(ctx); err != nil {
			_, _ = app.ErrWriter.Write([]byte(err.Error()))
		}
	}(ctx)

	ctx = config.WithConfigValues(ctx)
	// configure engine context
	ctx = engine.WithEngineValues(ctx)

	defer func(ctx context.Context) {
		if err := engine.Shutdown(ctx, app.opts); err != nil {
			_, _ = app.ErrWriter.Write([]byte(err.Error()))
		}
	}(ctx)

	args = removeNoColorFlagDuplicates(args)

	if err := app.App.RunContext(ctx, args); err != nil && !errors.IsContextCanceled(err) {
		return err
	}

	return nil
}

// removeNoColorFlagDuplicates removes one of the `--no-color` or `--terragrunt-no-color` arguments if both are present.
// We have to do this because `--terragrunt-no-color` is a deprecated alias for `--no-color`,
// therefore we end up specifying the same flag twice, which causes the `setting the flag multiple times` error.
func removeNoColorFlagDuplicates(args []string) []string {
	var ( //nolint:prealloc
		foundNoColor bool
		filteredArgs []string
	)

	for _, arg := range args {
		if strings.HasSuffix(arg, "-"+flags.NoColorFlagName) {
			if foundNoColor {
				continue
			}

			foundNoColor = true
		}

		filteredArgs = append(filteredArgs, arg)
	}

	return filteredArgs
}

// TerragruntCommands returns the set of Terragrunt commands.
func TerragruntCommands(opts *options.TerragruntOptions) cli.Commands {
	cmds := cli.Commands{
		runall.NewCommand(opts),             // runAction-all
		terragruntinfo.NewCommand(opts),     // terragrunt-info
		validateinputs.NewCommand(opts),     // validate-inputs
		graphdependencies.NewCommand(opts),  // graph-dependencies
		hclfmt.NewCommand(opts),             // hclfmt
		renderjson.NewCommand(opts),         // render-json
		awsproviderpatch.NewCommand(opts),   // aws-provider-patch
		outputmodulegroups.NewCommand(opts), // output-module-groups
		catalog.NewCommand(opts),            // catalog
		scaffold.NewCommand(opts),           // scaffold
		graph.NewCommand(opts),              // graph
		hclvalidate.NewCommand(opts),        // hclvalidate
		execCmd.NewCommand(opts),            // exec
	}

	sort.Sort(cmds)

	// add terraform command `*` after sorting to put the command at the end of the list in the help.
	cmds.Add(runCmd.NewCommand(opts))

	return cmds
}

// WrapWithTelemetry wraps CLI command execution with setting of telemetry context and labels, if telemetry is disabled, just runAction the command.
func WrapWithTelemetry(opts *options.TerragruntOptions) func(ctx *cli.Context, action cli.ActionFunc) error {
	return func(ctx *cli.Context, action cli.ActionFunc) error {
		return telemetry.Telemetry(ctx.Context, opts, fmt.Sprintf("%s %s", ctx.Command.Name, opts.TerraformCommand), map[string]interface{}{
			"terraformCommand": opts.TerraformCommand,
			"args":             opts.TerraformCliArgs,
			"dir":              opts.WorkingDir,
		}, func(childCtx context.Context) error {
			ctx.Context = childCtx //nolint:fatcontext
			if err := initialSetup(ctx, opts); err != nil {
				return err
			}

			// TODO: See if this lint should be ignored
			return runAction(ctx, opts, action) //nolint:contextcheck
		})
	}
}

func beforeAction(_ *options.TerragruntOptions) cli.ActionFunc {
	return func(ctx *cli.Context) error {
		// setting current context to the options
		// show help if the args are not specified.
		if !ctx.Args().Present() {
			err := cli.ShowAppHelp(ctx)
			// exit the app
			return cli.NewExitError(err, 0)
		}

		return nil
	}
}

func runAction(cliCtx *cli.Context, opts *options.TerragruntOptions, action cli.ActionFunc) error {
	ctx, cancel := context.WithCancel(cliCtx.Context)
	defer cancel()

	errGroup, ctx := errgroup.WithContext(ctx)

	// Run provider cache server
	if opts.ProviderCache {
		server, err := InitProviderCacheServer(opts)
		if err != nil {
			return err
		}

		ln, err := server.Listen()
		if err != nil {
			return err
		}
		defer ln.Close() //nolint:errcheck

		cliCtx.Context = shell.ContextWithTerraformCommandHook(ctx, server.TerraformCommandHook)

		errGroup.Go(func() error {
			return server.Run(ctx, ln)
		})
	}

	// Run command action
	errGroup.Go(func() error {
		defer cancel()

		if action != nil {
			return action(cliCtx)
		}

		return nil
	})

	return errGroup.Wait()
}

// mostly preparing terragrunt options
func initialSetup(cliCtx *cli.Context, opts *options.TerragruntOptions) error {
	// The env vars are renamed to "..._NO_AUTO_..." in the global flags`. These ones are left for backwards compatibility.
	opts.AutoInit = env.GetBool(os.Getenv("TERRAGRUNT_AUTO_INIT"), opts.AutoInit)
	opts.AutoRetry = env.GetBool(os.Getenv("TERRAGRUNT_AUTO_RETRY"), opts.AutoRetry)
	opts.RunAllAutoApprove = env.GetBool(os.Getenv("TERRAGRUNT_AUTO_APPROVE"), opts.RunAllAutoApprove)

	// `TF_INPUT` is the old env var for`--terragrunt-non-interactive` flag, now is replaced with `TERRAGRUNT_NON_INTERACTIVE` but kept for backwards compatibility.
	// If `TF_INPUT` is false then `opts.NonInteractive` is true.
	opts.NonInteractive = env.GetNegativeBool(os.Getenv("TF_INPUT"), opts.NonInteractive)

	// --- Args
	// convert the rest flags (intended for terraform) to one dash, e.g. `--input=true` to `-input=true`
	args := cliCtx.Args().WithoutEndOfFlagsSign().Normalize(cli.SingleDashFlag)
	cmdName := cliCtx.Command.Name

	switch cmdName {
	case runCmd.CommandName, runall.CommandName, graph.CommandName:
		cmdName = cliCtx.Args().CommandName()

		// `terraform apply -destroy` is an alias for `terraform destroy`.
		// It is important to resolve the alias because the `run-all` relies on terraform command to determine the order, for `destroy` command is used the reverse order.
		if cmdName == terraform.CommandNameApply && util.ListContainsElement(args, terraform.FlagNameDestroy) {
			cmdName = terraform.CommandNameDestroy
			args = append([]string{terraform.CommandNameDestroy}, args.Tail()...)
			args = util.RemoveElementFromList(args, terraform.FlagNameDestroy)
		}
	default:
		args = append([]string{cmdName}, args...)
	}

	// Since Terragrunt and Terraform have the same `-no-color` flag,
	// if a user specifies `-no-color` for Terragrunt, we should propagate it to Terraform as well.
	if opts.DisableLogColors {
		args = append(args, terraform.FlagNameNoColor)
	}

	opts.TerraformCommand = cmdName
	opts.TerraformCliArgs = args

	opts.Env = env.Parse(os.Environ())

	// --- Working Dir
	if opts.WorkingDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return errors.New(err)
		}

		opts.WorkingDir = currentDir
	}

	opts.WorkingDir = filepath.ToSlash(opts.WorkingDir)

	workingDir, err := filepath.Abs(opts.WorkingDir)
	if err != nil {
		return errors.New(err)
	}

	opts.Logger = opts.Logger.WithField(placeholders.WorkDirKeyName, workingDir)

	opts.RootWorkingDir = filepath.ToSlash(workingDir)

	if err := opts.LogFormatter.SetBaseDir(opts.RootWorkingDir); err != nil {
		return err
	}

	if opts.LogShowAbsPaths {
		opts.LogFormatter.DisableRelativePaths()
	}

	// --- Download Dir
	if opts.DownloadDir == "" {
		opts.DownloadDir = util.JoinPath(opts.WorkingDir, util.TerragruntCacheDir)
	}

	downloadDir, err := filepath.Abs(opts.DownloadDir)
	if err != nil {
		return errors.New(err)
	}

	opts.DownloadDir = filepath.ToSlash(downloadDir)

	// --- Terragrunt ConfigPath
	if opts.TerragruntConfigPath == "" {
		opts.TerragruntConfigPath = config.GetDefaultConfigPath(opts.WorkingDir)
	} else if !filepath.IsAbs(opts.TerragruntConfigPath) && cliCtx.Command.Name == runCmd.CommandName {
		opts.TerragruntConfigPath = util.JoinPath(opts.WorkingDir, opts.TerragruntConfigPath)
	}

	opts.TerragruntConfigPath, err = filepath.Abs(opts.TerragruntConfigPath)
	if err != nil {
		return errors.New(err)
	}

	opts.TerraformPath = filepath.ToSlash(opts.TerraformPath)

	opts.ExcludeDirs, err = util.GlobCanonicalPath(opts.WorkingDir, opts.ExcludeDirs...)
	if err != nil {
		return err
	}

	if len(opts.IncludeDirs) > 0 {
		opts.Logger.Debugf("Included directories set. Excluding by default.")
		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && len(opts.ModulesThatInclude) > 0 {
		opts.Logger.Debugf("Modules that include set. Excluding by default.")
		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && len(opts.UnitsReading) > 0 {
		opts.Logger.Debugf("Units that read set. Excluding by default.")
		opts.ExcludeByDefault = true
	}

	if !opts.ExcludeByDefault && opts.StrictInclude {
		opts.Logger.Debugf("Strict include set. Excluding by default.")
		opts.ExcludeByDefault = true
	}

	opts.IncludeDirs, err = util.GlobCanonicalPath(opts.WorkingDir, opts.IncludeDirs...)
	if err != nil {
		return err
	}

	excludeDirs, err := util.GetExcludeDirsFromFile(opts.WorkingDir, opts.ExcludesFile)
	if err != nil {
		return err
	}

	opts.ExcludeDirs = append(opts.ExcludeDirs, excludeDirs...)

	// --- Terragrunt Version
	terragruntVersion, err := hashicorpversion.NewVersion(cliCtx.App.Version)
	if err != nil {
		// Malformed Terragrunt version; set the version to 0.0
		if terragruntVersion, err = hashicorpversion.NewVersion("0.0"); err != nil {
			return errors.New(err)
		}
	}

	opts.TerragruntVersion = terragruntVersion
	// Log the terragrunt version in debug mode. This helps with debugging issues and ensuring a specific version of terragrunt used.
	opts.Logger.Debugf("Terragrunt Version: %s", opts.TerragruntVersion)

	// --- Others
	if !opts.RunAllAutoApprove {
		// When running in no-auto-approve mode, set parallelism to 1 so that interactive prompts work.
		opts.Parallelism = 1
	}

	opts.OriginalTerragruntConfigPath = opts.TerragruntConfigPath
	opts.OriginalTerraformCommand = opts.TerraformCommand
	opts.OriginalIAMRoleOptions = opts.IAMRoleOptions

	opts.RunTerragrunt = runCmd.Run

	exec.PrepareConsole(opts.Logger)

	return nil
}

// OSExiter is an empty function that overrides the default behavior.
func OSExiter(exitCode int) {
	// Do nothing. We just need to override this function, as the default value calls os.Exit, which
	// kills the app (or any automated test) dead in its tracks.
}

// ExitErrHandler is an empty function that overrides the default behavior.
func ExitErrHandler(_ *cli.Context, err error) error {
	return err
}
