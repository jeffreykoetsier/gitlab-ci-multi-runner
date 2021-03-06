package parallels

import (
	"errors"
	"os/exec"

	"github.com/ayufan/gitlab-ci-multi-runner/common"
	"github.com/ayufan/gitlab-ci-multi-runner/executors"
	"github.com/ayufan/gitlab-ci-multi-runner/ssh"

	prl "github.com/ayufan/gitlab-ci-multi-runner/parallels"

	"time"
)

type ParallelsExecutor struct {
	executors.AbstractExecutor
	cmd             *exec.Cmd
	vmName          string
	sshCommand      ssh.SshCommand
	provisioned     bool
	ipAddress       string
	machineVerified bool
}

func (s *ParallelsExecutor) waitForIpAddress(vmName string, seconds int) (string, error) {
	var lastError error

	if s.ipAddress != "" {
		return s.ipAddress, nil
	}

	s.Debugln("Looking for MAC address...")
	macAddr, err := prl.Mac(vmName)
	if err != nil {
		return "", err
	}

	s.Debugln("Requesting IP address...")
	for i := 0; i < seconds; i++ {
		ipAddr, err := prl.IpAddress(macAddr)
		if err == nil {
			s.Debugln("IP address found", ipAddr, "...")
			s.ipAddress = ipAddr
			return ipAddr, nil
		}
		lastError = err
		time.Sleep(time.Second)
	}
	return "", lastError
}

func (s *ParallelsExecutor) verifyMachine(vmName string) error {
	if s.machineVerified {
		return nil
	}

	ipAddr, err := s.waitForIpAddress(vmName, 120)
	if err != nil {
		return err
	}

	// Create SSH command
	sshCommand := ssh.SshCommand{
		SshConfig:      *s.Config.Ssh,
		Command:        "exit 0",
		Stdout:         s.BuildLog,
		Stderr:         s.BuildLog,
		ConnectRetries: 30,
	}
	sshCommand.Host = ipAddr

	s.Debugln("Connecting to SSH...")
	err = sshCommand.Connect()
	if err != nil {
		return err
	}
	defer sshCommand.Cleanup()
	err = sshCommand.Run()
	if err != nil {
		return err
	}
	s.machineVerified = true
	return nil
}

func (s *ParallelsExecutor) restoreFromSnapshot() error {
	s.Debugln("Requesting default snapshot for VM...")
	snapshot, err := prl.GetDefaultSnapshot(s.vmName)
	if err != nil {
		return err
	}

	s.Debugln("Reverting VM to snapshot", snapshot, "...")
	err = prl.RevertToSnapshot(s.vmName, snapshot)
	if err != nil {
		return err
	}

	return nil
}

func (s *ParallelsExecutor) createVm() error {
	baseImage := s.Config.Parallels.BaseName
	if baseImage == "" {
		return errors.New("Missing Image setting from Parallels config")
	}

	templateName := s.Config.Parallels.TemplateName
	if templateName == "" {
		templateName = baseImage + "-template"
	}

	if !prl.Exist(templateName) {
		s.Debugln("Creating template from VM", baseImage, "...")
		err := prl.CreateTemplate(baseImage, templateName)
		if err != nil {
			return err
		}
	}

	s.Debugln("Creating runner from VM template...")
	err := prl.CreateOsVm(s.vmName, templateName)
	if err != nil {
		return err
	}

	s.Debugln("Bootstraping VM...")
	err = prl.Start(s.vmName)
	if err != nil {
		return err
	}

	s.Debugln("Waiting for VM to start...")
	err = prl.TryExec(s.vmName, 120, "exit", "0")
	if err != nil {
		return err
	}

	s.Debugln("Waiting for VM to become responsive...")
	err = s.verifyMachine(s.vmName)
	if err != nil {
		return err
	}
	return nil
}

func (s *ParallelsExecutor) Prepare(config *common.RunnerConfig, build *common.Build) error {
	err := s.AbstractExecutor.Prepare(config, build)
	if err != nil {
		return err
	}

	if s.Config.Ssh == nil {
		return errors.New("Missing SSH configuration")
	}

	if s.Config.Parallels == nil {
		return errors.New("Missing Parallels configuration")
	}

	if s.Config.Parallels.BaseName == "" {
		return errors.New("Missing BaseName setting from Parallels config")
	}

	version, err := prl.Version()
	if err != nil {
		return err
	}

	s.Debugln("Using Parallels", version, "executor...")

	if s.Config.Parallels.DisableSnapshots {
		s.vmName = s.Config.Parallels.BaseName + "-" + s.Build.ProjectRunnerName
		if prl.Exist(s.vmName) {
			s.Debugln("Deleting old VM...")
			prl.Stop(s.vmName)
			prl.Delete(s.vmName)
		}
	} else {
		s.vmName = s.Build.RunnerName
	}

	if prl.Exist(s.vmName) {
		s.Debugln("Restoring VM from snapshot...")
		err := s.restoreFromSnapshot()
		if err != nil {
			return err
		}
	} else {
		s.Debugln("Creating new VM...")
		err := s.createVm()
		if err != nil {
			return err
		}

		if !s.Config.Parallels.DisableSnapshots {
			s.Debugln("Creating default snapshot...")
			err = prl.CreateSnapshot(s.vmName, "Started")
			if err != nil {
				return err
			}
		}
	}

	s.Debugln("Checking VM status...")
	status, err := prl.Status(s.vmName)
	if err != nil {
		return err
	}

	// Start VM if stopped
	if status == "stopped" || status == "suspended" {
		s.Debugln("Starting VM...")
		err := prl.Start(s.vmName)
		if err != nil {
			return err
		}
	}

	if status != "running" {
		s.Debugln("Waiting for VM to run...")
		err = prl.WaitForStatus(s.vmName, "running", 60)
		if err != nil {
			return err
		}
	}

	s.Debugln("Waiting VM to become responsive...")
	err = s.verifyMachine(s.vmName)
	if err != nil {
		return err
	}

	s.provisioned = true

	s.Debugln("Updating VM date...")
	err = prl.TryExec(s.vmName, 20, "sudo", "ntpdate", "-u", "time.apple.com")
	if err != nil {
		return err
	}
	return nil
}

func (s *ParallelsExecutor) Start() error {
	ipAddr, err := s.waitForIpAddress(s.vmName, 60)
	if err != nil {
		return err
	}

	s.Debugln("Starting SSH command...")
	s.sshCommand = ssh.SshCommand{
		SshConfig:   *s.Config.Ssh,
		Environment: append(s.Build.GetEnv(), s.Config.Environment...),
		Command:     "bash",
		Stdin:       s.BuildScript,
		Stdout:      s.BuildLog,
		Stderr:      s.BuildLog,
	}
	s.sshCommand.Host = ipAddr

	s.Debugln("Connecting to SSH server...")
	err = s.sshCommand.Connect()
	if err != nil {
		return err
	}

	// Wait for process to exit
	go func() {
		s.Debugln("Will run SSH command...")
		err := s.sshCommand.Run()
		s.Debugln("SSH command finished with", err)
		s.BuildFinish <- err
	}()
	return nil
}

func (s *ParallelsExecutor) Cleanup() {
	s.sshCommand.Cleanup()

	if s.vmName != "" {
		prl.Kill(s.vmName)

		if s.Config.Parallels.DisableSnapshots || !s.provisioned {
			prl.Delete(s.vmName)
		}
	}

	s.AbstractExecutor.Cleanup()
}

func init() {
	common.RegisterExecutor("parallels", func() common.Executor {
		return &ParallelsExecutor{
			AbstractExecutor: executors.AbstractExecutor{
				DefaultBuildsDir: "tmp/builds",
				ShowHostname:     true,
			},
		}
	})
}
