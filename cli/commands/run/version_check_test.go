//nolint:unparam
package run_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Terraform Version Checking
func TestCheckTerraformVersionMeetsConstraintEqual(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v0.9.3", ">= v0.9.3", true)
}

func TestCheckTerraformVersionMeetsConstraintGreaterPatch(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v0.9.4", ">= v0.9.3", true)
}

func TestCheckTerraformVersionMeetsConstraintGreaterMajor(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v1.0.0", ">= v0.9.3", true)
}

func TestCheckTerraformVersionMeetsConstraintLessPatch(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v0.9.2", ">= v0.9.3", false)
}

func TestCheckTerraformVersionMeetsConstraintLessMajor(t *testing.T) {
	t.Parallel()
	testCheckTerraformVersionMeetsConstraint(t, "v0.8.8", ">= v0.9.3", false)
}

func TestParseOpenTofuVersionNormal(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "OpenTofu v1.6.0", "v1.6.0", nil)
}

func TestParseOpenTofuVersionDev(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "OpenTofu v1.6.0-dev", "v1.6.0", nil)
}

func TestParseTerraformVersionNormal(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.9.3", "v0.9.3", nil)
}

func TestParseTerraformVersionWithoutV(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform 0.9.3", "0.9.3", nil)
}

func TestParseTerraformVersionWithDebug(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.9.4 cad024a5fe131a546936674ef85445215bbc4226", "v0.9.4", nil)
}

func TestParseTerraformVersionWithChanges(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.9.4-dev (cad024a5fe131a546936674ef85445215bbc4226+CHANGES)", "v0.9.4", nil)
}

func TestParseTerraformVersionWithDev(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.9.4-dev", "v0.9.4", nil)
}

func TestParseTerraformVersionWithBeta(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.13.0-beta1", "v0.13.0", nil)
}

func TestParseTerraformVersionWithUnexpectedName(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "Terraform v0.15.0-rc1", "v0.15.0", nil)
}

func TestParseTerraformVersionInvalidSyntax(t *testing.T) {
	t.Parallel()
	testParseTerraformVersion(t, "invalid-syntax", "", run.InvalidTerraformVersionSyntax("invalid-syntax"))
}

func testCheckTerraformVersionMeetsConstraint(t *testing.T, currentVersion string, versionConstraint string, versionMeetsConstraint bool) {
	t.Helper()

	current, err := version.NewVersion(currentVersion)
	if err != nil {
		t.Fatalf("Invalid current version specified in test: %v", err)
	}

	err = run.CheckTerraformVersionMeetsConstraint(current, versionConstraint)
	if versionMeetsConstraint && err != nil {
		assert.NoError(t, err, "Expected Terraform version %s to meet constraint %s, but got error: %v", currentVersion, versionConstraint, err)
	} else if !versionMeetsConstraint && err == nil {
		assert.Error(t, err, "Expected Terraform version %s to NOT meet constraint %s, but got back a nil error", currentVersion, versionConstraint)
	}
}

func testParseTerraformVersion(t *testing.T, versionString string, expectedVersion string, expectedErr error) {
	t.Helper()

	actualVersion, actualErr := run.ParseTerraformVersion(versionString)
	if expectedErr == nil {
		expected, err := version.NewVersion(expectedVersion)
		if err != nil {
			t.Fatalf("Invalid expected version specified in test: %v", err)
		}

		require.NoError(t, actualErr)
		assert.Equal(t, expected, actualVersion)
	} else {
		assert.True(t, errors.IsError(actualErr, expectedErr))
	}
}

// TODO: Refactor these into a test table.

// Terragrunt Version Checking
func TestCheckTerragruntVersionMeetsConstraintEqual(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.23.18", ">= v0.23.18", true)
}

func TestCheckTerragruntVersionMeetsConstraintGreaterPatch(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.23.18", ">= v0.23.9", true)
}

func TestCheckTerragruntVersionMeetsConstraintGreaterMajor(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v1.0.0", ">= v0.23.18", true)
}

func TestCheckTerragruntVersionMeetsConstraintLessPatch(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.23.17", ">= v0.23.18", false)
}

func TestCheckTerragruntVersionMeetsConstraintLessMajor(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.22.15", ">= v0.23.18", false)
}

func TestCheckTerragruntVersionMeetsConstraintPrerelease(t *testing.T) {
	t.Parallel()
	testCheckTerragruntVersionMeetsConstraint(t, "v0.23.18-alpha202409013", ">= v0.23.18", true)
}

func testCheckTerragruntVersionMeetsConstraint(t *testing.T, currentVersion string, versionConstraint string, versionMeetsConstraint bool) {
	t.Helper()

	current, err := version.NewVersion(currentVersion)
	if err != nil {
		t.Fatalf("Invalid current version specified in test: %v", err)
	}

	err = run.CheckTerragruntVersionMeetsConstraint(current, versionConstraint)
	if versionMeetsConstraint && err != nil {
		t.Fatalf("Expected Terragrunt version %s to meet constraint %s, but got error: %v", currentVersion, versionConstraint, err)
	} else if !versionMeetsConstraint && err == nil {
		t.Fatalf("Expected Terragrunt version %s to NOT meet constraint %s, but got back a nil error", currentVersion, versionConstraint)
	}
}
