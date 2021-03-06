package docker

import (
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsouza/go-dockerclient"

	"github.com/ayufan/gitlab-ci-multi-runner/common"
	"github.com/ayufan/gitlab-ci-multi-runner/executors"
)

type DockerExecutor struct {
	executors.AbstractExecutor
	client    *docker.Client
	image     *docker.Image
	container *docker.Container
	services  []*docker.Container
}

func (s *DockerExecutor) volumeDir(cache_dir string, project_name string, volume string) string {
	hash := md5.Sum([]byte(volume))
	return fmt.Sprintf("%s/%s/%x", cache_dir, project_name, hash)
}

func (s *DockerExecutor) getImage(imageName string, pullImage bool) (*docker.Image, error) {
	s.Debugln("Looking for image", imageName, "...")
	image, err := s.client.InspectImage(imageName)
	if err == nil {
		return image, nil
	}

	if !pullImage {
		return nil, err
	}

	s.Println("Pulling docker image", imageName, "...")
	pull_image_opts := docker.PullImageOptions{
		Repository: imageName,
		Registry:   s.Config.Docker.Registry,
	}

	err = s.client.PullImage(pull_image_opts, docker.AuthConfiguration{})
	if err != nil {
		return nil, err
	}

	return s.client.InspectImage(imageName)
}

func (s *DockerExecutor) addVolume(binds *[]string, cache_dir string, volume string) {
	volumeDir := s.volumeDir(cache_dir, s.Build.ProjectUniqueName(), volume)
	*binds = append(*binds, fmt.Sprintf("%s:%s:rw", volumeDir, volume))
	s.Debugln("Using", volumeDir, "for", volume, "...")

	// TODO: this is potentially insecure
	os.MkdirAll(volumeDir, 0777)
}

func (s *DockerExecutor) createVolumes(image *docker.Image, builds_dir string) ([]string, error) {
	cache_dir := "tmp/docker-cache"
	if len(s.Config.Docker.CacheDir) != 0 {
		cache_dir = s.Config.Docker.CacheDir
	}

	cache_dir, err := filepath.Abs(cache_dir)
	if err != nil {
		return nil, err
	}

	var binds []string

	for _, volume := range s.Config.Docker.Volumes {
		s.addVolume(&binds, cache_dir, volume)
	}

	if image != nil {
		for volume, _ := range image.Config.Volumes {
			s.addVolume(&binds, cache_dir, volume)
		}
	}

	if s.Build.AllowGitFetch {
		s.addVolume(&binds, cache_dir, builds_dir)
	}

	return binds, nil
}

func (s *DockerExecutor) splitServiceAndVersion(service string) (string, string) {
	splits := strings.SplitN(service, ":", 2)
	switch len(splits) {
	case 1:
		return splits[0], "latest"

	case 2:
		return splits[0], splits[1]

	default:
		return "", ""
	}
}

func (s *DockerExecutor) createService(service, version string) (*docker.Container, error) {
	if len(service) == 0 {
		return nil, errors.New("Invalid service name")
	}

	serviceImage, err := s.getImage(service+":"+version, !s.Config.Docker.DisablePull)
	if err != nil {
		return nil, err
	}

	containerName := s.Build.ProjectUniqueName() + "-" + strings.Replace(service, "/", "__", -1)

	// this will fail potentially some builds if there's name collision
	s.removeContainer(containerName)

	createContainerOpts := docker.CreateContainerOptions{
		Name: containerName,
		Config: &docker.Config{
			Image: serviceImage.ID,
			Env:   s.Config.Environment,
		},
		HostConfig: &docker.HostConfig{
			RestartPolicy: docker.NeverRestart(),
		},
	}

	s.Debugln("Creating service container", createContainerOpts.Name, "...")
	container, err := s.client.CreateContainer(createContainerOpts)
	if err != nil {
		return nil, err
	}

	s.Debugln("Starting service container", container.ID, "...")
	err = s.client.StartContainer(container.ID, createContainerOpts.HostConfig)
	if err != nil {
		go s.removeContainer(container.ID)
		return nil, err
	}

	return container, nil
}

func (s *DockerExecutor) createServices() ([]string, error) {
	var links []string

	for _, serviceDescription := range s.Config.Docker.Services {
		service, version := s.splitServiceAndVersion(serviceDescription)
		container, err := s.createService(service, version)
		if err != nil {
			return links, err
		}

		s.Debugln("Created service", service, version, "as", container.ID)
		links = append(links, container.Name+":"+strings.Replace(service, "/", "__", -1))
		s.services = append(s.services, container)
	}

	return links, nil
}

func (s *DockerExecutor) connect() (*docker.Client, error) {
	endpoint := s.Config.Docker.Host
	if len(endpoint) == 0 {
		endpoint = os.Getenv("DOCKER_HOST")
	}
	if len(endpoint) == 0 {
		endpoint = "unix:///var/run/docker.sock"
	}
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (s *DockerExecutor) createContainer(image *docker.Image, cmd []string) (*docker.Container, error) {
	hostname := s.Config.Docker.Hostname
	if hostname == "" {
		hostname = s.Build.ProjectUniqueName()
	}

	containerName := s.Build.ProjectUniqueName()

	// this will fail potentially some builds if there's name collision
	s.removeContainer(containerName)

	create_container_opts := docker.CreateContainerOptions{
		Name: containerName,
		Config: &docker.Config{
			Hostname:     hostname,
			Image:        image.ID,
			Tty:          false,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			OpenStdin:    true,
			StdinOnce:    true,
			Env:          append(s.Build.GetEnv(), s.Config.Environment...),
			Cmd:          cmd,
		},
		HostConfig: &docker.HostConfig{
			Privileged:    s.Config.Docker.Privileged,
			RestartPolicy: docker.NeverRestart(),
			ExtraHosts:    s.Config.Docker.ExtraHosts,
			Links:         s.Config.Docker.Links,
		},
	}

	s.Debugln("Creating services...")
	links, err := s.createServices()
	if err != nil {
		return nil, err
	}
	create_container_opts.HostConfig.Links = append(create_container_opts.HostConfig.Links, links...)

	if !s.Config.Docker.DisableCache {
		s.Debugln("Creating cache directories...")
		binds, err := s.createVolumes(image, s.BuildsDir)
		if err != nil {
			return nil, err
		}
		create_container_opts.HostConfig.Binds = binds
	}

	s.Debugln("Creating container", create_container_opts.Name, "...")
	container, err := s.client.CreateContainer(create_container_opts)
	if err != nil {
		if container != nil {
			go s.removeContainer(container.ID)
		}
		return nil, err
	}

	s.Debugln("Starting container", container.ID, "...")
	err = s.client.StartContainer(container.ID, create_container_opts.HostConfig)
	if err != nil {
		go s.removeContainer(container.ID)
		return nil, err
	}

	return container, nil
}

func (s *DockerExecutor) removeContainer(id string) error {
	remove_container_opts := docker.RemoveContainerOptions{
		ID:            id,
		RemoveVolumes: true,
		Force:         true,
	}
	err := s.client.RemoveContainer(remove_container_opts)
	s.Debugln("Removed container", id, "with", err)
	return err
}

func (s *DockerExecutor) Prepare(config *common.RunnerConfig, build *common.Build) error {
	err := s.AbstractExecutor.Prepare(config, build)
	if err != nil {
		return err
	}

	s.Println("Using Docker executor with image", s.Config.Docker.Image, "...")

	if config.Docker == nil {
		return errors.New("Missing docker configuration")
	}

	client, err := s.connect()
	if err != nil {
		return err
	}
	s.client = client

	// Get image
	image, err := s.getImage(s.Config.Docker.Image, !s.Config.Docker.DisablePull)
	if err != nil {
		return err
	}
	s.image = image
	return nil
}

func (s *DockerExecutor) Cleanup() {
	for _, service := range s.services {
		s.removeContainer(service.ID)
	}

	if s.container != nil {
		s.removeContainer(s.container.ID)
		s.container = nil
	}

	s.AbstractExecutor.Cleanup()
}
