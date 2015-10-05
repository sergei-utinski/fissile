package compilator

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hpcloud/fissile/docker"
	"github.com/hpcloud/fissile/model"
	"github.com/hpcloud/fissile/scripts/compilation"

	"github.com/fatih/color"
	dockerClient "github.com/fsouza/go-dockerclient"
	"github.com/hashicorp/go-multierror"
	"github.com/termie/go-shutil"
)

const (
	ContainerPackagesDir = "/var/vcap/packages"

	sleepTimeWhileCantCompileSec = 5
)

const (
	packageError = iota
	packageNone
	packageCompiling
	packageCompiled
)

type Compilator struct {
	DockerManager    *docker.DockerImageManager
	Release          *model.Release
	HostWorkDir      string
	DockerRepository string
	BaseType         string

	packageLock      map[*model.Package]*sync.Mutex
	packageCompiling map[*model.Package]bool
}

func NewCompilator(
	dockerManager *docker.DockerImageManager,
	release *model.Release,
	hostWorkDir string,
	dockerRepository string,
	baseType string,
) (*Compilator, error) {

	compilator := &Compilator{
		DockerManager:    dockerManager,
		Release:          release,
		HostWorkDir:      hostWorkDir,
		DockerRepository: dockerRepository,
		BaseType:         baseType,

		packageLock:      map[*model.Package]*sync.Mutex{},
		packageCompiling: map[*model.Package]bool{},
	}

	for _, pkg := range release.Packages {
		compilator.packageLock[pkg] = &sync.Mutex{}
		compilator.packageCompiling[pkg] = false
	}

	return compilator, nil
}

func (c *Compilator) Compile(workerCount int) error {
	var result error

	// TODO Check for cycles

	// Iterate until all packages are compiled
	// Not the most efficient implementation,
	// but it's easy to parallelize and reason about
	var workersGroup sync.WaitGroup

	for idx := 0; idx < workerCount; idx++ {
		workersGroup.Add(1)

		go func(workerIdx int) {
			defer workersGroup.Done()

			hasWork := false
			done := false

			for done == false {
				log.Printf("worker-%s > Compilation work started.\n", color.YellowString("%d", workerIdx))

				hasWork = false

				for _, pkg := range c.Release.Packages {
					pkgState, workerErr := c.getPackageStatus(pkg)
					if workerErr != nil {
						result = multierror.Append(result, workerErr)
						return
					}

					if pkgState == packageNone {
						func() {
							func() {
								c.packageLock[pkg].Lock()
								c.packageCompiling[pkg] = true
								defer func() {
									c.packageLock[pkg].Unlock()
								}()
							}()
							defer func() {
								c.packageLock[pkg].Lock()
								c.packageCompiling[pkg] = false
								defer func() {
									c.packageLock[pkg].Unlock()
								}()
							}()
							if pkgState == packageNone {
								hasWork = true
								workerErr = c.compilePackage(pkg)
								if workerErr != nil {
									log.Println(color.RedString(
										"worker-%s > Compiling package %s failed: %s.\n",
										color.YellowString("%d", workerIdx),
										color.GreenString(pkg.Name),
										color.RedString(workerErr.Error()),
									))
									result = multierror.Append(result, workerErr)
								}
							}
						}()
						if result != nil {
							break
						}
					}
				}

				if result != nil {
					break
				}

				done = true

				for _, pkg := range c.Release.Packages {
					pkgState, workerErr := c.getPackageStatus(pkg)
					if workerErr != nil {
						result = multierror.Append(result, workerErr)
						return
					}

					if pkgState != packageCompiled {
						done = false
						break
					}
				}

				// Wait a bit if there's nothing to work on
				if !done && !hasWork {
					log.Printf("worker-%s > Didn't find any work, sleeping ...\n", color.YellowString("%d", workerIdx))
					time.Sleep(sleepTimeWhileCantCompileSec * time.Second)
				}
			}
		}(idx)
	}

	// Wait until all workers finish
	workersGroup.Wait()

	return result
}

func (c *Compilator) CreateCompilationBase(baseImageName string) (image *dockerClient.Image, err error) {
	imageTag := c.BaseCompilationImageTag()
	imageName := fmt.Sprintf("%s:%s", c.DockerRepository, imageTag)
	log.Println(color.GreenString("Using %s as a compilation image name", color.YellowString(imageName)))

	containerName := c.BaseCompilationContainerName()
	log.Println(color.GreenString("Using %s as a compilation container name", color.YellowString(containerName)))

	image, err = c.DockerManager.FindImage(imageName)
	if err != nil {
		log.Println("Image doesn't exist, it will be created ...")
	} else {
		log.Println(color.GreenString(
			"Compilation image %s with ID %s already exists. Doing nothing.",
			color.YellowString(imageName),
			color.YellowString(image.ID),
		))
		return image, nil
	}

	tempScriptDir, err := ioutil.TempDir("", "fissile-compilation")
	if err != nil {
		return nil, fmt.Errorf("Could not create temp dir %s: %s", tempScriptDir, err.Error())
	}

	targetScriptName := "compilation-prerequisites.sh"
	containerScriptPath := filepath.Join(docker.ContainerInPath, targetScriptName)
	hostScriptPath := filepath.Join(tempScriptDir, targetScriptName)
	if err = compilation.SaveScript(c.BaseType, compilation.PreprequisitesScript, hostScriptPath); err != nil {
		return nil, fmt.Errorf("Error saving script asset: %s", err.Error())
	}

	exitCode, container, err := c.DockerManager.RunInContainer(
		containerName,
		baseImageName,
		[]string{"bash", "-c", containerScriptPath},
		tempScriptDir,
		"",
		func(stdout io.Reader) {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				log.Println(color.GreenString("compilation-container > %s", color.WhiteString(scanner.Text())))
			}
		},
		func(stderr io.Reader) {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				log.Println(color.GreenString("compilation-container > %s", color.RedString(scanner.Text())))
			}
		},
	)
	defer func() {
		if container != nil {
			removeErr := c.DockerManager.RemoveContainer(container.ID)
			if removeErr != nil {
				if err == nil {
					err = removeErr
				} else {
					err = fmt.Errorf(
						"Image creation error: %s. Image removal error: %s.",
						err,
						removeErr,
					)
				}
			}
		}
	}()

	if err != nil {
		return nil, fmt.Errorf("Error running script: %s", err.Error())
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("Error - script script exited with code %d", exitCode)
	}

	image, err = c.DockerManager.CreateImage(
		container.ID,
		c.DockerRepository,
		imageTag,
		"",
		[]string{},
	)

	if err != nil {
		return nil, fmt.Errorf("Error creating image %s", err.Error())
	}

	log.Println(color.GreenString(
		"Image %s:%s with ID %s created successfully.",
		color.YellowString(c.DockerRepository),
		color.YellowString(imageTag),
		color.YellowString(container.ID)))

	return image, nil
}

func (c *Compilator) compilePackage(pkg *model.Package) (err error) {

	// Do nothing if any dependency has not been compiled
	for _, dep := range pkg.Dependencies {

		packageStatus, err := c.getPackageStatus(dep)
		if err != nil {
			return err
		}

		if packageStatus != packageCompiled {
			return nil
		}
	}
	log.Println(color.GreenString("compilation-%s > %s", color.MagentaString(pkg.Name), color.WhiteString("Starting compilation ...")))

	// Prepare input dir (package plus deps)
	if err := c.createCompilationDirStructure(pkg); err != nil {
		return err
	}

	if err := c.copyDependencies(pkg); err != nil {
		return err
	}

	// Generate a compilation script
	targetScriptName := "compile.sh"
	hostScriptPath := filepath.Join(c.getTargetPackageSourcesDir(pkg), targetScriptName)
	containerScriptPath := filepath.Join(docker.ContainerInPath, targetScriptName)
	if err := compilation.SaveScript(c.BaseType, compilation.CompilationScript, hostScriptPath); err != nil {
		return err
	}

	// Extract package
	extractDir := c.getSourcePackageDir(pkg)
	if _, err := pkg.Extract(extractDir); err != nil {
		return err
	}

	// Run compilation in container
	containerName := c.getPackageContainerName(pkg)
	exitCode, container, err := c.DockerManager.RunInContainer(
		containerName,
		fmt.Sprintf("%s:%s", c.DockerRepository, c.BaseCompilationImageTag()),
		[]string{"bash", containerScriptPath, pkg.Name, pkg.Version},
		c.getTargetPackageSourcesDir(pkg),
		c.getPackageCompiledDir(pkg),
		func(stdout io.Reader) {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				log.Println(color.GreenString("compilation-%s > %s", color.MagentaString(pkg.Name), color.WhiteString(scanner.Text())))
			}
		},
		func(stderr io.Reader) {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				log.Println(color.GreenString("compilation-%s > %s", color.MagentaString(pkg.Name), color.RedString(scanner.Text())))
			}
		},
	)
	defer func() {
		// Remove container
		if container != nil {
			if removeErr := c.DockerManager.RemoveContainer(container.ID); removeErr != nil {
				if err == nil {
					err = removeErr
				} else {
					err = fmt.Errorf("Error compiling package: %s. Error removing package: %s", err.Error(), removeErr.Error())
				}
			}
		}
	}()

	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("Error - compilation for package %s exited with code %d", pkg.Name, exitCode)
	}

	return nil
}

// We want to create a package structure like this:
// .
// └── <pkg-name>
//     ├── compiled
//     └── sources
//         └── var
//             └── vcap
//                 ├── packages
//                 │   └── <dependency-package>
//                 └── source
func (c *Compilator) createCompilationDirStructure(pkg *model.Package) error {
	dependenciesPackageDir := c.getDependenciesPackageDir(pkg)
	sourcePackageDir := c.getSourcePackageDir(pkg)

	if err := os.MkdirAll(dependenciesPackageDir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(sourcePackageDir, 0755); err != nil {
		return err
	}

	return nil
}

func (c *Compilator) getTargetPackageSourcesDir(pkg *model.Package) string {
	return filepath.Join(c.HostWorkDir, pkg.Name, "sources")
}

func (c *Compilator) getPackageCompiledDir(pkg *model.Package) string {
	return filepath.Join(c.HostWorkDir, pkg.Name, "compiled")
}

func (c *Compilator) getDependenciesPackageDir(pkg *model.Package) string {
	return filepath.Join(c.getTargetPackageSourcesDir(pkg), "var", "vcap", "packages")
}

func (c *Compilator) getSourcePackageDir(pkg *model.Package) string {
	return filepath.Join(c.getTargetPackageSourcesDir(pkg), "var", "vcap", "source")
}

func (c *Compilator) copyDependencies(pkg *model.Package) error {
	for _, dep := range pkg.Dependencies {
		depCompiledPath := c.getPackageCompiledDir(dep)
		depDestinationPath := filepath.Join(c.getDependenciesPackageDir(pkg), dep.Name)
		if err := os.RemoveAll(depDestinationPath); err != nil {
			return err
		}

		if err := shutil.CopyTree(depCompiledPath, depDestinationPath, nil); err != nil {
			return err
		}
	}

	return nil
}

func (c *Compilator) getPackageStatus(pkg *model.Package) (int, error) {
	// Acquire mutex before checking status
	c.packageLock[pkg].Lock()
	defer func() {
		c.packageLock[pkg].Unlock()
	}()

	// If package is in packageCompiling hash
	if c.packageCompiling[pkg] {
		return packageCompiling, nil
	}

	// If compiled package exists on hard
	compiledPackagePath := filepath.Join(c.HostWorkDir, pkg.Name, "compiled")
	compiledPackagePathExists, err := validatePath(compiledPackagePath, true, "package path")
	if err != nil {
		return packageError, err
	}

	if compiledPackagePathExists {
		compiledDirEmpty, err := isDirEmpty(compiledPackagePath)
		if err != nil {
			return packageError, err
		}

		if !compiledDirEmpty {
			return packageCompiled, nil
		}
	}

	// Package is in no state otherwise
	return packageNone, nil
}

func isDirEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return true, err
	}

	defer f.Close()

	_, err = f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}

	return false, err
}

func validatePath(path string, shouldBeDir bool, pathDescription string) (bool, error) {
	pathInfo, err := os.Stat(path)

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	}

	if pathInfo.IsDir() && !shouldBeDir {
		return false, fmt.Errorf("Path %s (%s) points to a directory. It should be a a file. %s", path, pathDescription)
	} else if !pathInfo.IsDir() && shouldBeDir {
		return false, fmt.Errorf("Path %s (%s) points to a file. It should be a directory.", path, pathDescription)
	}

	return true, nil
}

func (c *Compilator) getPackageContainerName(pkg *model.Package) string {
	return fmt.Sprintf("%s-%s-%s-pkg-%s", c.DockerRepository, c.Release.Name, c.Release.Version, pkg.Name)
}

func (c *Compilator) BaseCompilationContainerName() string {
	return fmt.Sprintf("%s-%s-%s-cbase", c.DockerRepository, c.Release.Name, c.Release.Version)
}

func (c *Compilator) BaseCompilationImageTag() string {
	return fmt.Sprintf("%s-%s-cbase", c.Release.Name, c.Release.Version)
}