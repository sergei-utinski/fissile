package model

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestLoadRoleManifestOK(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/tor-good.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths:    []string{torReleasePath},
		ReleaseNames:    []string{},
		ReleaseVersions: []string{},
		BOSHCacheDir:    filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.NoError(t, err)
	require.NotNil(t, roleManifest)

	assert.Equal(t, roleManifestPath, roleManifest.manifestFilePath)
	assert.Len(t, roleManifest.InstanceGroups, 2)

	myrole := roleManifest.InstanceGroups[0]
	assert.Equal(t, []string{
		"scripts/myrole.sh",
		"/script/with/absolute/path.sh",
	}, myrole.Scripts)

	foorole := roleManifest.InstanceGroups[1]
	torjob := foorole.JobReferences[0]
	assert.Equal(t, "tor", torjob.Name)
	assert.NotNil(t, torjob.Release)
	assert.Equal(t, "tor", torjob.Release.Name)
}

func TestScriptPathInvalid(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/script-bad-prefix.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})
	require.Error(t, err, "invalid role manifest should return error")
	assert.Nil(t, roleManifest, "invalid role manifest loaded")
	for _, msg := range []string{
		`myrole environment script: Invalid value: "lacking-prefix.sh": Script path does not start with scripts/`,
		`myrole script: Invalid value: "scripts/missing.sh": script not found`,
		`myrole post config script: Invalid value: "": script not found`,
	} {
		assert.Contains(t, err.Error(), msg, "missing expected validation error")
	}
	for _, msg := range []string{
		`myrole environment script: Invalid value: "scripts/environ.sh":`,
		`myrole environment script: Invalid value: "/environ/script/with/absolute/path.sh":`,
		`myrole script: Invalid value: "scripts/myrole.sh":`,
		`myrole script: Invalid value: "/script/with/absolute/path.sh":`,
		`myrole post config script: Invalid value: "scripts/post_config_script.sh":`,
		`myrole post config script: Invalid value: "/var/vcap/jobs/myrole/pre-start":`,
		`myrole post config script: Invalid value: "scripts/nested/run.sh":`,
		`scripts/nested: Required value: Script is not used`,
	} {
		assert.NotContains(t, err.Error(), msg, "unexpected validation error")
	}
}

func TestLoadRoleManifestNotOKBadJobName(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/tor-bad.yml")
	_, err = LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "Cannot find job foo in release")
	}
}

func TestLoadDuplicateReleases(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/tor-good.yml")
	_, err = LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "release tor has been loaded more than once")
	}
}

func TestLoadRoleManifestMultipleReleasesOK(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/multiple-good.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, ntpReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.NoError(t, err)
	require.NotNil(t, roleManifest)

	assert.Equal(t, roleManifestPath, roleManifest.manifestFilePath)
	assert.Len(t, roleManifest.InstanceGroups, 2)

	myrole := roleManifest.InstanceGroups[0]
	assert.Len(t, myrole.Scripts, 1)
	assert.Equal(t, "scripts/myrole.sh", myrole.Scripts[0])

	foorole := roleManifest.InstanceGroups[1]
	torjob := foorole.JobReferences[0]
	assert.Equal(t, "tor", torjob.Name)
	if assert.NotNil(t, torjob.Release) {
		assert.Equal(t, "tor", torjob.Release.Name)
	}
}

func TestLoadRoleManifestMultipleReleasesNotOk(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/multiple-bad.yml")
	_, err = LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, ntpReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})

	if assert.Error(t, err) {
		assert.Contains(t, err.Error(),
			`instance_groups[foorole].jobs[ntpd]: Invalid value: "foo": Referenced release is not loaded`)
	}
}

func TestRoleManifestTagList(t *testing.T) {
	t.Parallel()
	workDir, err := os.Getwd()
	require.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	releases, err := LoadReleases(
		[]string{torReleasePath},
		[]string{},
		[]string{},
		filepath.Join(workDir, "../test-assets/bosh-cache"))
	require.NoError(t, err, "Error reading BOSH release")

	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/tor-good.yml")
	manifestContents, err := ioutil.ReadFile(roleManifestPath)
	require.NoError(t, err, "Error reading role manifest")

	for tag, acceptableRoleTypes := range map[string][]RoleType{
		"stop-on-failure":    []RoleType{RoleTypeBoshTask},
		"sequential-startup": []RoleType{RoleTypeBosh},
		"active-passive":     []RoleType{RoleTypeBosh},
		"indexed":            []RoleType{},
		"clustered":          []RoleType{},
		"invalid":            []RoleType{},
		"no-monit":           []RoleType{},
	} {
		for _, roleType := range []RoleType{RoleTypeBosh, RoleTypeBoshTask, RoleTypeColocatedContainer} {
			func(tag string, roleType RoleType, acceptableRoleTypes []RoleType) {
				t.Run(tag, func(t *testing.T) {
					t.Parallel()
					roleManifest := &RoleManifest{
						manifestFilePath: roleManifestPath,
						validationOptions: RoleManifestValidationOptions{
							AllowMissingScripts: true,
						}}
					roleManifest.LoadedReleases = releases
					err := yaml.Unmarshal(manifestContents, roleManifest)
					require.NoError(t, err, "Error unmarshalling role manifest")
					roleManifest.Configuration = &Configuration{Templates: yaml.MapSlice{}}
					require.NotEmpty(t, roleManifest.InstanceGroups, "No instance groups loaded")
					roleManifest.InstanceGroups[0].Type = roleType
					roleManifest.InstanceGroups[0].Tags = []RoleTag{RoleTag(tag)}
					if RoleTag(tag) == RoleTagActivePassive {
						// An active/passive probe is required when tagged as active/passive
						roleManifest.InstanceGroups[0].JobReferences[0].ContainerProperties.BoshContainerization.Run = &RoleRun{ActivePassiveProbe: "hello"}
					}
					err = roleManifest.resolveRoleManifest(nil)
					acceptable := false
					for _, acceptableRoleType := range acceptableRoleTypes {
						if acceptableRoleType == roleType {
							acceptable = true
						}
					}
					if acceptable {
						assert.NoError(t, err)
					} else {
						message := "Unknown tag"
						if len(acceptableRoleTypes) > 0 {
							var roleNames []string
							for _, acceptableRoleType := range acceptableRoleTypes {
								roleNames = append(roleNames, string(acceptableRoleType))
							}
							message = fmt.Sprintf("%s tag is only supported in [%s] instance groups, not %s",
								tag,
								strings.Join(roleNames, ", "),
								roleType)
						}
						fullMessage := fmt.Sprintf(`instance_groups[myrole].tags[0]: Invalid value: "%s": %s`, tag, message)
						assert.EqualError(t, err, fullMessage)
					}
				})
			}(tag, roleType, acceptableRoleTypes)
		}
	}
}

func TestNonBoshRolesAreNotAllowed(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/non-bosh-roles.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})
	assert.EqualError(t, err, "instance_groups[dockerrole].type: Invalid value: \"docker\": Expected one of bosh, bosh-task, or colocated-container")
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestVariablesSortedError(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/variables-badly-sorted.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})
	require.Error(t, err)

	assert.Contains(t, err.Error(), `variables: Invalid value: "FOO": Does not sort before 'BAR'`)
	assert.Contains(t, err.Error(), `variables: Invalid value: "PELERINUL": Does not sort before 'ALPHA'`)
	assert.Contains(t, err.Error(), `variables: Invalid value: "PELERINUL": Appears more than once`)
	// Note how this ignores other errors possibly present in the manifest and releases.
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestVariablesPreviousNamesError(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/variables-with-dup-prev-names.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})
	require.Error(t, err)

	assert.Contains(t, err.Error(), `variables: Invalid value: "FOO": Previous name 'BAR' also exist as a new variable`)
	assert.Contains(t, err.Error(), `variables: Invalid value: "FOO": Previous name 'BAZ' also claimed by 'QUX'`)
	assert.Contains(t, err.Error(), `variables: Invalid value: "QUX": Previous name 'BAZ' also claimed by 'FOO'`)
	// Note how this ignores other errors possibly present in the manifest and releases.
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestVariablesNotUsed(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/variables-without-usage.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.EqualError(t, err,
		`variables: Not found: "No templates using 'SOME_VAR'"`)
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestVariablesNotDeclared(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/variables-without-decl.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.EqualError(t, err,
		`variables: Not found: "No declaration of 'HOME'"`)
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestVariablesSSH(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/variables-ssh.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})

	assert.NoError(t, err)
	assert.NotNil(t, roleManifest)
}

func TestLoadRoleManifestNonTemplates(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/templates-non.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.EqualError(t, err,
		`properties.tor.hostname: Forbidden: Templates used as constants are not allowed`)
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestBadType(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/bad-type.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})

	require.Contains(t, err.Error(),
		`variables[BAR].type: Invalid value: "invalid": Expected one of certificate, password, rsa, ssh or empty`)
	require.Contains(t, err.Error(),
		`variables[FOO].type: Invalid value: "rsa": The rsa type is not yet supported by the secret generator`)
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestBadCVType(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/bad-cv-type.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})

	require.EqualError(t, err,
		`variables[BAR].options.type: Invalid value: "bogus": Expected one of user, or environment`)
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestBadCVTypeConflictInternal(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/bad-cv-type-internal.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.EqualError(t, err,
		`variables[BAR].options.type: Invalid value: "environment": type conflicts with flag "internal"`)
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestMissingRBACAccount(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/rbac-missing-account.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})
	assert.EqualError(t, err, `instance_groups[myrole].run.service-account: Not found: "missing-account"`)
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestMissingRBACRole(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/rbac-missing-role.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.EqualError(t, err, `configuration.auth.accounts[test-account].roles: Not found: "missing-role"`)
	assert.Nil(t, roleManifest)
}

func TestLoadRoleManifestPSPMerge(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir,
		"../test-assets/role-manifests/model/rbac-merge-psp.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.NoError(t, err)

	// The loaded manifest has a single role with two jobs,
	// requesting differing psps (The second role's request is
	// implicit, the default).  The sole service account ends up
	// with the union of the two, the higher. This is the account
	// referenced by the role

	assert.NotNil(t, roleManifest)
	assert.NotNil(t, roleManifest.Configuration)

	assert.Len(t, roleManifest.InstanceGroups, 1)
	assert.NotNil(t, roleManifest.InstanceGroups[0])

	assert.Len(t, roleManifest.InstanceGroups[0].JobReferences, 2)
	assert.NotNil(t, roleManifest.InstanceGroups[0].JobReferences[0])
	assert.NotNil(t, roleManifest.InstanceGroups[0].JobReferences[1])

	assert.NotNil(t, roleManifest.InstanceGroups[0].Run)
	assert.Equal(t, "default", roleManifest.InstanceGroups[0].Run.ServiceAccount)

	// manifest value
	assert.Equal(t, "privileged", roleManifest.InstanceGroups[0].JobReferences[0].ContainerProperties.BoshContainerization.PodSecurityPolicy)

	// default
	assert.Equal(t, "nonprivileged", roleManifest.InstanceGroups[0].JobReferences[1].ContainerProperties.BoshContainerization.PodSecurityPolicy)

	assert.NotNil(t, roleManifest.Configuration.Authorization.Accounts)
	assert.Len(t, roleManifest.Configuration.Authorization.Accounts, 1)
	assert.Contains(t, roleManifest.Configuration.Authorization.Accounts, "default")
	assert.Equal(t, "privileged", roleManifest.Configuration.Authorization.Accounts["default"].PodSecurityPolicy)
}

func TestLoadRoleManifestSACloneForPSPMismatch(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	roleManifestPath := filepath.Join(workDir,
		"../test-assets/role-manifests/model/rbac-clone-sa-psp-mismatch.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.NoError(t, err)

	// The loaded manifest has three roles with one job each. All
	// reference the same service account (default), while
	// requesting differing psps. This results in two service
	// accounts, the second a clone of the first, but for the psp.
	// Note how the third role (re)uses the account created by the
	// second which did the cloning.

	assert.NotNil(t, roleManifest)
	assert.NotNil(t, roleManifest.Configuration)

	assert.Len(t, roleManifest.InstanceGroups, 3)
	assert.NotNil(t, roleManifest.InstanceGroups[0])
	assert.NotNil(t, roleManifest.InstanceGroups[1])
	assert.NotNil(t, roleManifest.InstanceGroups[2])

	assert.Len(t, roleManifest.InstanceGroups[0].JobReferences, 1)
	assert.Len(t, roleManifest.InstanceGroups[1].JobReferences, 1)
	assert.Len(t, roleManifest.InstanceGroups[2].JobReferences, 1)

	assert.NotNil(t, roleManifest.InstanceGroups[0].JobReferences[0])
	assert.NotNil(t, roleManifest.InstanceGroups[1].JobReferences[0])
	assert.NotNil(t, roleManifest.InstanceGroups[2].JobReferences[0])

	assert.Equal(t, "privileged", roleManifest.InstanceGroups[0].JobReferences[0].ContainerProperties.BoshContainerization.PodSecurityPolicy)
	assert.Equal(t, "nonprivileged", roleManifest.InstanceGroups[1].JobReferences[0].ContainerProperties.BoshContainerization.PodSecurityPolicy)
	assert.Equal(t, "nonprivileged", roleManifest.InstanceGroups[2].JobReferences[0].ContainerProperties.BoshContainerization.PodSecurityPolicy)

	assert.NotNil(t, roleManifest.InstanceGroups[0].Run)
	assert.NotNil(t, roleManifest.InstanceGroups[1].Run)
	assert.NotNil(t, roleManifest.InstanceGroups[2].Run)

	assert.Equal(t, "default", roleManifest.InstanceGroups[0].Run.ServiceAccount)
	assert.Equal(t, "default-nonprivileged", roleManifest.InstanceGroups[1].Run.ServiceAccount)
	assert.Equal(t, "default-nonprivileged", roleManifest.InstanceGroups[2].Run.ServiceAccount)

	assert.NotNil(t, roleManifest.Configuration.Authorization.Accounts)
	assert.Len(t, roleManifest.Configuration.Authorization.Accounts, 2)

	assert.Contains(t, roleManifest.Configuration.Authorization.Accounts, "default")
	assert.Contains(t, roleManifest.Configuration.Authorization.Accounts, "default-nonprivileged")
	assert.Equal(t, "privileged", roleManifest.Configuration.Authorization.Accounts["default"].PodSecurityPolicy)
	assert.Equal(t, "nonprivileged", roleManifest.Configuration.Authorization.Accounts["default-nonprivileged"].PodSecurityPolicy)
}

func TestLoadRoleManifestRunGeneral(t *testing.T) {
	t.Parallel()

	workDir, err := os.Getwd()
	require.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")

	type testCase struct {
		manifest string
		message  []string
	}

	tests := []testCase{
		{
			"rbac-illegal-psp.yml", []string{
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.pod-security-policy: Invalid value: "bogus": Expected one of: nonprivileged, privileged`,
			},
		},
		{
			"rbac-ok-psp.yml", []string{},
		},
		{
			"bosh-run-missing.yml", []string{
				"instance_groups[myrole]: Required value: `properties.bosh_containerization.run` required for at least one Job",
			},
		},
		{
			"bosh-run-bad-proto.yml", []string{
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[https].protocol: Unsupported value: "AA": supported values: TCP, UDP`,
			},
		},
		{
			"bosh-run-bad-port-names.yml", []string{
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[a--b].name: Invalid value: "a--b": port names must be lowercase words separated by hyphens`,
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[abcd-efgh-ijkl-x].name: Invalid value: "abcd-efgh-ijkl-x": port name must be no more than 15 characters`,
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[abcdefghij].name: Invalid value: "abcdefghij": user configurable port name must be no more than 9 characters`,
			},
		},
		{
			"bosh-run-bad-port-count.yml", []string{
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[http].count: Invalid value: 2: count doesn't match port range 80-82`,
			},
		},
		{
			"bosh-run-bad-ports.yml", []string{
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[https].internal: Invalid value: "-1": invalid syntax`,
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[https].external: Invalid value: 0: must be between 1 and 65535, inclusive`,
			},
		},
		{
			"bosh-run-missing-portrange.yml", []string{
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[https].internal: Invalid value: "": invalid syntax`,
			},
		},
		{
			"bosh-run-reverse-portrange.yml", []string{
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[https].internal: Invalid value: "5678-123": last port can't be lower than first port`,
			},
		},
		{
			"bosh-run-bad-parse.yml", []string{
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[https].internal: Invalid value: "qq": invalid syntax`,
				`instance_groups[myrole].jobs[tor].properties.bosh_containerization.ports[https].external: Invalid value: "aa": invalid syntax`,
			},
		},
		{
			"bosh-run-bad-memory.yml", []string{
				`instance_groups[myrole].run.memory: Invalid value: -10: must be greater than or equal to 0`,
			},
		},
		{
			"bosh-run-bad-cpu.yml", []string{
				`instance_groups[myrole].run.virtual-cpus: Invalid value: -2: must be greater than or equal to 0`,
			},
		},
		{
			"bosh-run-ok.yml", []string{},
		},
	}

	for _, tc := range tests {
		func(tc testCase) {
			t.Run(tc.manifest, func(t *testing.T) {
				t.Parallel()
				roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model", tc.manifest)
				roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
					ReleasePaths: []string{torReleasePath},
					BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
					ValidationOptions: RoleManifestValidationOptions{
						AllowMissingScripts: true,
					}})

				if len(tc.message) > 0 {
					assert.EqualError(t, err, strings.Join(tc.message, "\n"))
					assert.Nil(t, roleManifest)
				} else {
					assert.NoError(t, err)
				}
			})
		}(tc)
	}
}

func TestLoadRoleManifestHealthChecks(t *testing.T) {
	t.Parallel()
	workDir, err := os.Getwd()
	require.NoError(t, err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	release, err := NewDevRelease(torReleasePath, "", "", filepath.Join(workDir, "../test-assets/bosh-cache"))
	require.NoError(t, err, "Error reading BOSH release")

	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/tor-good.yml")
	manifestContents, err := ioutil.ReadFile(roleManifestPath)
	require.NoError(t, err, "Error reading role manifest")

	type sampleStruct struct {
		name        string
		roleType    RoleType
		healthCheck HealthCheck
		err         []string
	}
	for _, sample := range []sampleStruct{
		{
			name: "empty",
		},
		{
			name:     "bosh task with health check",
			roleType: RoleTypeBoshTask,
			healthCheck: HealthCheck{
				Readiness: &HealthProbe{
					Command: []string{"hello"},
				},
			},
			err: []string{
				`instance_groups[myrole].run.healthcheck.readiness: Forbidden: bosh-task instance groups cannot have health checks`,
			},
		},
		{
			name:     "bosh role with command",
			roleType: RoleTypeBosh,
			healthCheck: HealthCheck{
				Readiness: &HealthProbe{
					Command: []string{"/bin/echo", "hello"},
				},
			},
		},
		{
			name:     "bosh role with url",
			roleType: RoleTypeBosh,
			healthCheck: HealthCheck{
				Readiness: &HealthProbe{
					URL: "about:crashes",
				},
			},
			err: []string{
				`instance_groups[myrole].run.healthcheck.readiness: Invalid value: ["url"]: Only command health checks are supported for BOSH instance groups`,
			},
		},
		{
			name:     "bosh role with liveness check with multiple commands",
			roleType: RoleTypeBosh,
			healthCheck: HealthCheck{
				Liveness: &HealthProbe{
					Command: []string{"hello", "world"},
				},
			},
			err: []string{
				`instance_groups[myrole].run.healthcheck.liveness.command: Invalid value: ["hello","world"]: liveness check can only have one command`,
			},
		},
	} {
		func(sample sampleStruct) {
			t.Run(sample.name, func(t *testing.T) {
				var err error // Do not share err with parallel invocations
				t.Parallel()
				roleManifest := &RoleManifest{
					manifestFilePath: roleManifestPath,
					validationOptions: RoleManifestValidationOptions{
						AllowMissingScripts: true,
					},
				}
				roleManifest.LoadedReleases = []*Release{release}
				err = yaml.Unmarshal(manifestContents, roleManifest)
				require.NoError(t, err, "Error unmarshalling role manifest")
				roleManifest.Configuration = &Configuration{Templates: yaml.MapSlice{}}
				require.NotEmpty(t, roleManifest.InstanceGroups, "No instance groups loaded")
				if sample.roleType != RoleType("") {
					roleManifest.InstanceGroups[0].Type = sample.roleType
				}
				roleManifest.InstanceGroups[0].JobReferences[0].ContainerProperties.BoshContainerization.Run = &RoleRun{
					HealthCheck: &sample.healthCheck,
				}
				err = roleManifest.resolveRoleManifest(nil)
				if len(sample.err) > 0 {
					assert.EqualError(t, err, strings.Join(sample.err, "\n"))
					return
				}
				assert.NoError(t, err)
			})
		}(sample)
	}

	t.Run("bosh role with untagged active/passive probe", func(t *testing.T) {
		t.Parallel()
		roleManifest := &RoleManifest{manifestFilePath: roleManifestPath}
		roleManifest.LoadedReleases = []*Release{release}
		err := yaml.Unmarshal(manifestContents, roleManifest)
		require.NoError(t, err, "Error unmarshalling role manifest")
		roleManifest.Configuration = &Configuration{Templates: yaml.MapSlice{}}
		require.NotEmpty(t, roleManifest.InstanceGroups, "No instance groups loaded")

		roleManifest.InstanceGroups[0].Type = RoleTypeBosh
		roleManifest.InstanceGroups[0].Tags = []RoleTag{}
		roleManifest.InstanceGroups[0].JobReferences[0].ContainerProperties.BoshContainerization.Run = &RoleRun{
			ActivePassiveProbe: "/bin/true",
		}
		err = roleManifest.resolveRoleManifest(nil)
		assert.EqualError(t, err,
			`instance_groups[myrole].run.active-passive-probe: Invalid value: "/bin/true": Active/passive probes are only valid on instance groups with active-passive tag`)
	})

	t.Run("active/passive bosh role without a probe", func(t *testing.T) {
		t.Parallel()
		roleManifest := &RoleManifest{manifestFilePath: roleManifestPath}
		roleManifest.LoadedReleases = []*Release{release}
		err := yaml.Unmarshal(manifestContents, roleManifest)
		require.NoError(t, err, "Error unmarshalling role manifest")
		roleManifest.Configuration = &Configuration{Templates: yaml.MapSlice{}}
		require.NotEmpty(t, roleManifest.InstanceGroups, "No instance groups loaded")

		roleManifest.InstanceGroups[0].Type = RoleTypeBosh
		roleManifest.InstanceGroups[0].Tags = []RoleTag{RoleTagActivePassive}
		roleManifest.InstanceGroups[0].JobReferences[0].ContainerProperties.BoshContainerization.Run = &RoleRun{}
		err = roleManifest.resolveRoleManifest(nil)
		assert.EqualError(t, err,
			`instance_groups[myrole].run.active-passive-probe: Required value: active-passive instance groups must specify the correct probe`)
	})

	t.Run("bosh task tagged as active/passive", func(t *testing.T) {
		t.Parallel()
		roleManifest := &RoleManifest{manifestFilePath: roleManifestPath}
		roleManifest.LoadedReleases = []*Release{release}
		err := yaml.Unmarshal(manifestContents, roleManifest)
		require.NoError(t, err, "Error unmarshalling role manifest")
		roleManifest.Configuration = &Configuration{Templates: yaml.MapSlice{}}
		require.NotEmpty(t, roleManifest.InstanceGroups, "No instance groups loaded")

		roleManifest.InstanceGroups[0].Type = RoleTypeBoshTask
		roleManifest.InstanceGroups[0].Tags = []RoleTag{RoleTagActivePassive}
		roleManifest.InstanceGroups[0].JobReferences[0].ContainerProperties.BoshContainerization.Run = &RoleRun{ActivePassiveProbe: "/bin/false"}
		err = roleManifest.resolveRoleManifest(nil)
		assert.EqualError(t, err,
			`instance_groups[myrole].tags[0]: Invalid value: "active-passive": active-passive tag is only supported in [bosh] instance groups, not bosh-task`)
	})
}

func TestResolveLinks(t *testing.T) {
	workDir, err := os.Getwd()

	assert.NoError(t, err)

	releasePaths := []string{}

	for _, dirName := range []string{"ntp-release", "tor-boshrelease"} {
		releasePath := filepath.Join(workDir, "../test-assets", dirName)
		releasePaths = append(releasePaths, releasePath)
	}

	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/multiple-good.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: releasePaths,
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.NoError(t, err)
	require.NotNil(t, roleManifest)

	// LoadRoleManifest implicitly runs resolveLinks()

	role := roleManifest.LookupInstanceGroup("myrole")
	job := role.LookupJob("ntpd")
	if !assert.NotNil(t, job) {
		return
	}

	// Comparing things with assert.Equal() just gives us impossible-to-read dumps
	samples := []struct {
		Name     string
		Type     string
		Optional bool
		Missing  bool
	}{
		// These should match the order in the ntp-release ntp job.MF
		{Name: "ntp-server", Type: "ntpd"},
		{Name: "ntp-client", Type: "ntp"},
		{Type: "missing", Missing: true},
	}

	expectedLength := 0
	for _, expected := range samples {
		t.Run("", func(t *testing.T) {
			if expected.Missing {
				for name, consumeInfo := range job.ResolvedConsumers {
					assert.NotEqual(t, expected.Type, consumeInfo.Type,
						"link should not resolve, got %s (type %s) in %s / %s",
						name, consumeInfo.Type, consumeInfo.RoleName, consumeInfo.JobName)
				}
				return
			}
			expectedLength++
			require.Contains(t, job.ResolvedConsumers, expected.Name, "link %s is missing", expected.Name)
			actual := job.ResolvedConsumers[expected.Name]
			assert.Equal(t, expected.Name, actual.Name, "link name mismatch")
			assert.Equal(t, expected.Type, actual.Type, "link type mismatch")
			assert.Equal(t, role.Name, actual.RoleName, "link role name mismatch")
			assert.Equal(t, job.Name, actual.JobName, "link job name mismatch")
		})
	}
	assert.Len(t, job.ResolvedConsumers, expectedLength)
}

func TestRoleResolveLinksMultipleProvider(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	job1 := &Job{
		Name: "job-1",
		AvailableProviders: map[string]jobProvidesInfo{
			"job-1-provider-1": {
				jobLinkInfo: jobLinkInfo{
					Name: "job-1-provider-1",
					Type: "link-1",
				},
			},
			"job-1-provider-2": {
				jobLinkInfo: jobLinkInfo{
					Name: "job-1-provider-2",
					Type: "link-2",
				},
			},
			"job-1-provider-3": {
				jobLinkInfo: jobLinkInfo{
					Name: "job-1-provider-3",
					Type: "link-5",
				},
			},
		},
		DesiredConsumers: []jobConsumesInfo{
			{
				jobLinkInfo: jobLinkInfo{
					Name: "job-1-provider-1",
					Type: "link-1",
				},
			},
		},
	}

	job2 := &Job{
		Name: "job-2",
		AvailableProviders: map[string]jobProvidesInfo{
			"job-2-provider-1": {
				jobLinkInfo: jobLinkInfo{
					Name: "job-2-provider-1",
					Type: "link-3",
				},
			},
		},
	}

	job3 := &Job{
		Name: "job-3",
		AvailableProviders: map[string]jobProvidesInfo{
			"job-3-provider-3": {
				jobLinkInfo: jobLinkInfo{
					Name: "job-3-provider-3",
					Type: "link-4",
				},
			},
		},
		DesiredConsumers: []jobConsumesInfo{
			{
				// There is exactly one implicit provider of this type; use it
				jobLinkInfo: jobLinkInfo{
					Type: "link-1", // j1
				},
			},
			{
				// This job has multiple available implicit providers with
				// the same type; this should not resolve.
				jobLinkInfo: jobLinkInfo{
					Type: "link-3", // j3
				},
				Optional: true,
			},
			{
				// There is exactly one explicit provider of this name
				jobLinkInfo: jobLinkInfo{
					Name: "job-3-provider-3", // j3
				},
			},
			{
				// There are no providers of this type
				jobLinkInfo: jobLinkInfo{
					Type: "missing",
				},
				Optional: true,
			},
			{
				// This requires an alias
				jobLinkInfo: jobLinkInfo{
					Name: "actual-consumer-name",
				},
				Optional: true, // Not resolvable in role 3
			},
		},
	}

	roleManifest := &RoleManifest{
		InstanceGroups: InstanceGroups{
			&InstanceGroup{
				Name: "role-1",
				JobReferences: JobReferences{
					{
						Job: job1,
						ExportedProviders: map[string]jobProvidesInfo{
							"job-1-provider-3": jobProvidesInfo{
								Alias: "unique-alias",
							},
						},
						ContainerProperties: JobContainerProperties{
							BoshContainerization: JobBoshContainerization{
								ServiceName: "job-1-service",
							},
						},
					},
					{Job: job2},
				},
			},
			&InstanceGroup{
				Name: "role-2",
				JobReferences: JobReferences{
					{Job: job2},
					{
						Job: job3,
						// This has an explicitly exported provider
						ExportedProviders: map[string]jobProvidesInfo{
							"job-3-provider-3": jobProvidesInfo{},
						},
						ResolvedConsumers: map[string]jobConsumesInfo{
							"actual-consumer-name": jobConsumesInfo{
								Alias: "unique-alias",
							},
						},
					},
				},
			},
			&InstanceGroup{
				Name: "role-3",
				// This does _not_ have an explicitly exported provider
				JobReferences: JobReferences{{Job: job2}, {Job: job3}},
			},
		},
	}
	for _, r := range roleManifest.InstanceGroups {
		for _, jobReference := range r.JobReferences {
			jobReference.Name = jobReference.Job.Name
			if jobReference.ResolvedConsumers == nil {
				jobReference.ResolvedConsumers = make(map[string]jobConsumesInfo)
			}
		}
	}
	errors := roleManifest.resolveLinks()
	assert.Empty(errors)
	role := roleManifest.LookupInstanceGroup("role-2")
	require.NotNil(role, "Failed to find role")
	job := role.LookupJob("job-3")
	require.NotNil(job, "Failed to find job")
	consumes := job.ResolvedConsumers

	assert.Len(consumes, 3, "incorrect number of resulting link consumers")

	if assert.Contains(consumes, "job-1-provider-1", "failed to find role by type") {
		assert.Equal(jobConsumesInfo{
			jobLinkInfo: jobLinkInfo{
				Name:        "job-1-provider-1",
				Type:        "link-1",
				RoleName:    "role-1",
				JobName:     "job-1",
				ServiceName: "job-1-service",
			},
		}, consumes["job-1-provider-1"], "found incorrect role by type")
	}

	assert.NotContains(consumes, "job-3-provider-1",
		"should not automatically resolve consumers with multiple providers of the type")

	if assert.Contains(consumes, "job-3-provider-3", "did not find explicitly named provider") {
		assert.Equal(jobConsumesInfo{
			jobLinkInfo: jobLinkInfo{
				Name:        "job-3-provider-3",
				Type:        "link-4",
				RoleName:    "role-2",
				JobName:     "job-3",
				ServiceName: "role-2-job-3",
			},
		}, consumes["job-3-provider-3"], "did not find explicitly named provider")
	}

	if assert.Contains(consumes, "actual-consumer-name", "did not resolve consumer with alias") {
		assert.Equal(jobConsumesInfo{
			jobLinkInfo: jobLinkInfo{
				Name:        "job-1-provider-3",
				Type:        "link-5",
				RoleName:    "role-1",
				JobName:     "job-1",
				ServiceName: "job-1-service",
			},
		}, consumes["actual-consumer-name"], "resolved to incorrect provider for alias")
	}
}

func TestLoadRoleManifestColocatedContainers(t *testing.T) {
	assert := assert.New(t)

	workDir, err := os.Getwd()
	assert.NoError(err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/colocated-containers.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, ntpReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.NoError(err)
	assert.NotNil(roleManifest)

	assert.Len(roleManifest.InstanceGroups, 2)
	assert.EqualValues(RoleTypeBosh, roleManifest.LookupInstanceGroup("main-role").Type)
	assert.EqualValues(RoleTypeColocatedContainer, roleManifest.LookupInstanceGroup("to-be-colocated").Type)
	assert.Len(roleManifest.LookupInstanceGroup("main-role").ColocatedContainers(), 1)

	for _, roleName := range []string{"main-role", "to-be-colocated"} {
		assert.EqualValues([]*RoleRunVolume{&RoleRunVolume{Path: "/var/vcap/store", Type: "emptyDir", Tag: "shared-data"}}, roleManifest.LookupInstanceGroup(roleName).Run.Volumes)
	}
}

func TestLoadRoleManifestColocatedContainersValidationMissingRole(t *testing.T) {
	assert := assert.New(t)

	workDir, err := os.Getwd()
	assert.NoError(err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/colocated-containers-with-missing-role.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, ntpReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache")})
	assert.Nil(roleManifest)
	assert.EqualError(err, `instance_groups[main-role].colocated_containers[0]: Invalid value: "to-be-colocated-typo": There is no such instance group defined`)
}

func TestLoadRoleManifestColocatedContainersValidationUsusedRole(t *testing.T) {
	assert := assert.New(t)

	workDir, err := os.Getwd()
	assert.NoError(err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/colocated-containers-with-unused-role.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, ntpReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.Nil(roleManifest)
	assert.EqualError(err, "instance_group[to-be-colocated].job[ntpd].consumes[ntp-server]: Required value: failed to resolve provider ntp-server (type ntpd)\n"+
		"instance_group[orphaned].job[ntpd].consumes[ntp-server]: Required value: failed to resolve provider ntp-server (type ntpd)\n"+
		"instance_group[orphaned]: Not found: \"instance group is of type colocated container, but is not used by any other instance group as such\"")
}

func TestLoadRoleManifestColocatedContainersValidationPortCollisions(t *testing.T) {
	assert := assert.New(t)

	workDir, err := os.Getwd()
	assert.NoError(err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/colocated-containers-with-port-collision.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, ntpReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.Nil(roleManifest)
	assert.EqualError(err, "instance_group[main-role]: Invalid value: \"TCP/10443\": port collision, the same protocol/port is used by: main-role, to-be-colocated"+"\n"+
		"instance_group[main-role]: Invalid value: \"TCP/80\": port collision, the same protocol/port is used by: main-role, to-be-colocated")
}

func TestLoadRoleManifestColocatedContainersValidationPortCollisionsWithProtocols(t *testing.T) {
	assert := assert.New(t)

	workDir, err := os.Getwd()
	assert.NoError(err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/colocated-containers-with-no-port-collision.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, ntpReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.NoError(err)
	assert.NotNil(roleManifest)
}

func TestLoadRoleManifestColocatedContainersValidationInvalidTags(t *testing.T) {
	assert := assert.New(t)

	workDir, err := os.Getwd()
	assert.NoError(err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")

	_, err = NewDevRelease(torReleasePath, "", "", filepath.Join(workDir, "../test-assets/bosh-cache"))
	assert.NoError(err)

	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	_, err = NewDevRelease(ntpReleasePath, "", "", filepath.Join(workDir, "../test-assets/bosh-cache"))
	assert.NoError(err)
}

func TestLoadRoleManifestColocatedContainersValidationOfSharedVolumes(t *testing.T) {
	assert := assert.New(t)

	workDir, err := os.Getwd()
	assert.NoError(err)

	torReleasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	ntpReleasePath := filepath.Join(workDir, "../test-assets/ntp-release")
	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/colocated-containers-with-volume-share-issues.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		ReleasePaths: []string{torReleasePath, ntpReleasePath},
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.Nil(roleManifest)
	assert.EqualError(err, "instance_group[to-be-colocated]: Invalid value: \"/mnt/foobAr\": colocated instance group specifies a shared volume with tag mount-share, which path does not match the path of the main instance group shared volume with the same tag\n"+
		"instance_group[main-role]: Required value: container must use shared volumes of the main instance group: vcap-logs\n"+
		"instance_group[main-role]: Required value: container must use shared volumes of the main instance group: vcap-store")
}

func TestLoadRoleManifestWithReleaseReferences(t *testing.T) {
	workDir, err := os.Getwd()
	assert.NoError(t, err)

	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/model/online-release-references.yml")
	roleManifest, err := LoadRoleManifest(roleManifestPath, LoadRoleManifestOptions{
		BOSHCacheDir: filepath.Join(workDir, "../test-assets/bosh-cache"),
		ValidationOptions: RoleManifestValidationOptions{
			AllowMissingScripts: true,
		}})
	assert.NoError(t, err)
	require.NotNil(t, roleManifest)

	assert.Equal(t, roleManifestPath, roleManifest.manifestFilePath)
	assert.Len(t, roleManifest.InstanceGroups, 1)
}
