package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	dockerclient "github.com/fsouza/go-dockerclient"
)

const (
	// ContainerInPath is the input path for fissile
	ContainerInPath = "/fissile-in"
	// ContainerOutPath is the output path for fissile
	ContainerOutPath = "/fissile-out"
)

var (
	// ErrImageNotFound is the error returned when an image is not found.
	ErrImageNotFound = fmt.Errorf("Image not found")
)

// ProcessOutStream is stdout of the process
type ProcessOutStream func(io.Reader)

// ImageManager handles Docker images
type ImageManager struct {
	client *dockerclient.Client
}

// NewImageManager creates an instance of ImageManager
func NewImageManager() (*ImageManager, error) {
	manager := &ImageManager{}

	client, err := dockerclient.NewClientFromEnv()
	manager.client = client

	if err != nil {
		return nil, err
	}

	return manager, nil
}

// BuildImage builds a docker image using a directory that contains a Dockerfile
func (d *ImageManager) BuildImage(dockerfileDirPath, name string, stdoutProcessor ProcessOutStream) error {

	var stdoutReader io.ReadCloser
	var stdoutWriter io.WriteCloser

	if stdoutProcessor != nil {
		stdoutReader, stdoutWriter = io.Pipe()
	}

	if stdoutProcessor != nil {
		go func() {
			stdoutProcessor(stdoutReader)
		}()
	}

	bio := dockerclient.BuildImageOptions{
		Name:         name,
		NoCache:      true,
		ContextDir:   filepath.Dir(dockerfileDirPath),
		OutputStream: stdoutWriter,
	}

	if err := d.client.BuildImage(bio); err != nil {
		return err
	}

	if stdoutWriter != nil {
		stdoutWriter.Close()
	}

	if stdoutReader != nil {
		stdoutReader.Close()
	}

	return nil
}

// FindImage will lookup an image in Docker
func (d *ImageManager) FindImage(imageName string) (*dockerclient.Image, error) {
	image, err := d.client.InspectImage(imageName)

	if err == dockerclient.ErrNoSuchImage {
		return nil, ErrImageNotFound
	} else if err != nil {
		return nil, fmt.Errorf("Error looking up image %s: %s", imageName, err.Error())
	}

	return image, nil
}

// RemoveContainer will remove a container from Docker
func (d *ImageManager) RemoveContainer(containerID string) error {
	return d.client.RemoveContainer(dockerclient.RemoveContainerOptions{
		ID:    containerID,
		Force: true,
	})
}

// RemoveImage will remove an image from Docker's internal registry
func (d *ImageManager) RemoveImage(imageName string) error {
	return d.client.RemoveImage(imageName)
}

// CreateImage will create a Docker image
func (d *ImageManager) CreateImage(containerID string, repository string, tag string, message string, cmd []string) (*dockerclient.Image, error) {
	cco := dockerclient.CommitContainerOptions{
		Container:  containerID,
		Repository: repository,
		Tag:        tag,
		Author:     "fissile",
		Message:    message,
		Run: &dockerclient.Config{
			Cmd: cmd,
		},
	}

	return d.client.CommitContainer(cco)
}

// RunInContainer will execute a set of commands within a running Docker container
func (d *ImageManager) RunInContainer(containerName string, imageName string, cmd []string, inPath, outPath string, keepContainer bool, stdoutProcessor ProcessOutStream, stderrProcessor ProcessOutStream) (exitCode int, container *dockerclient.Container, err error) {
	exitCode = -1

	// Get current user info to map to container
	// os/user.Current() isn't supported when cross-compiling hence this code
	currentUID := syscall.Geteuid()
	currentGID := syscall.Getegid()
	var actualCmd, containerCmd []string
	if keepContainer {
		// Sleep effectively forever so if something goes wrong we can
		// docker exec -it bash into the container, investigate, and
		// manually kill the container. Most of the time the compile step
		// will succeed and the container will be killed and removed.
		containerCmd = []string{"sleep", "365d"}
		actualCmd = cmd
	} else {
		containerCmd = cmd
		// actualCmd not used
	}

	cco := dockerclient.CreateContainerOptions{
		Config: &dockerclient.Config{
			Tty:          false,
			AttachStdin:  false,
			AttachStdout: true,
			AttachStderr: true,
			Hostname:     "compiler",
			Domainname:   "fissile",
			Cmd:          containerCmd,
			WorkingDir:   "/",
			Image:        imageName,
			Env: []string{
				fmt.Sprintf("HOST_USERID=%d", currentUID),
				fmt.Sprintf("HOST_USERGID=%d", currentGID),
			},
		},
		HostConfig: &dockerclient.HostConfig{
			Privileged:     false,
			Binds:          []string{},
			ReadonlyRootfs: false,
		},
		Name: containerName,
	}

	if inPath != "" {
		cco.HostConfig.Binds = append(cco.HostConfig.Binds, fmt.Sprintf("%s:%s:ro", inPath, ContainerInPath))
	}

	if outPath != "" {
		cco.HostConfig.Binds = append(cco.HostConfig.Binds, fmt.Sprintf("%s:%s", outPath, ContainerOutPath))
	}

	container, err = d.client.CreateContainer(cco)
	if err != nil {
		return -1, nil, err
	}

	attached := make(chan struct{})

	var stdoutReader, stderrReader io.ReadCloser
	var stdoutWriter, stderrWriter io.WriteCloser

	if stdoutProcessor != nil {
		stdoutReader, stdoutWriter = io.Pipe()
	}

	if stderrProcessor != nil {
		stderrReader, stderrWriter = io.Pipe()
	}

	go func() {
		if attachErr := d.client.AttachToContainer(dockerclient.AttachToContainerOptions{
			Container: container.ID,

			InputStream:  nil,
			OutputStream: stdoutWriter,
			ErrorStream:  stderrWriter,

			Stdin:       false,
			Stdout:      stdoutProcessor != nil,
			Stderr:      stderrProcessor != nil,
			Stream:      true,
			RawTerminal: false,

			Success: attached,
		}); attachErr != nil {
			if err == nil {
				err = attachErr
			} else {
				err = fmt.Errorf("Error running in container: %s. Error attaching to container: %s", err.Error(), attachErr.Error())
			}
		}
	}()

	attached <- <-attached

	err = d.client.StartContainer(container.ID, container.HostConfig)
	if err != nil {
		return -1, container, err
	}

	var processorsGroup sync.WaitGroup
	runFileProcessors := func() {
		if stdoutProcessor != nil {
			processorsGroup.Add(1)

			go func() {
				defer processorsGroup.Done()
				stdoutProcessor(stdoutReader)
			}()
		}

		if stderrProcessor != nil {
			processorsGroup.Add(1)

			go func() {
				defer processorsGroup.Done()
				stderrProcessor(stderrReader)
			}()
		}
	}
	closeFiles := func() {
		if stdoutWriter != nil {
			stdoutWriter.Close()
		}
		if stderrWriter != nil {
			stderrWriter.Close()
		}

		if stdoutReader != nil {
			stdoutReader.Close()
		}
		if stderrReader != nil {
			stderrReader.Close()
		}
	}

	if !keepContainer {
		runFileProcessors()
		exitCode, err = d.client.WaitContainer(container.ID)
		closeFiles()
		if err != nil {
			return -1, container, err
		}

		processorsGroup.Wait()
		return exitCode, container, nil
	}
	// KeepContainer mode:
	// Run the cmd with 'docker exec ...' so we can keep the container around.
	// Note that this time we'll need to stop it if it doesn't fail
	cmdArgs := append([]string{"exec", "-i", container.ID}, actualCmd...)

	// Couldn't get this to work with dockerclient.Exec, so do it this way
	execCmd := exec.Command("docker", cmdArgs...)
	if stdoutProcessor != nil {
		stdoutReader, err = execCmd.StdoutPipe()
		if err != nil {
			return -1, container, err
		}
	}
	if stderrProcessor != nil {
		stderrReader, err = execCmd.StderrPipe()
		if err != nil {
			return -1, container, err
		}
	}
	stdin, err := execCmd.StdinPipe()
	if err != nil {
		return -1, container, err
	}
	err = execCmd.Start()
	if err != nil {
		return -1, container, err
	}
	runFileProcessors()
	err = execCmd.Wait()
	if err != nil {
		return -1, container, err
	}
	stdin.Close()
	closeFiles()
	processorsGroup.Wait()
	// Now we want to kill off this container?
	err = d.client.KillContainer(dockerclient.KillContainerOptions{
		ID: container.ID,
	})
	if err != nil {
		return -1, container, err
	}
	err = d.RemoveContainer(container.ID)
	if err != nil {
		// Return 0 anyway, as there's nothing we can look at.
		fmt.Fprintf(os.Stderr, "fissile: unexpected condition: client.RemoveContainer failed, err:%v\n", err)
	}
	return 0, container, nil
}
