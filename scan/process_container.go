package scan

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/deepfence/SecretScanner/core"
	"github.com/deepfence/SecretScanner/output"
	"github.com/deepfence/vessel"
	vesselConstants "github.com/deepfence/vessel/constants"
	containerdRuntime "github.com/deepfence/vessel/containerd"
	crioRuntime "github.com/deepfence/vessel/crio"
	dockerRuntime "github.com/deepfence/vessel/docker"
)

var (
	containerTarFileName = "save-output.tar"
)

type ContainerScan struct {
	containerId string
	tempDir     string
	namespace   string
	numSecrets  uint
}

// Function to retrieve contents of container
// @parameters
// containerScan - Structure with details of the container to scan
// @returns
// Error - Errors, if any. Otherwise, returns nil
func (containerScan *ContainerScan) extractFileSystem() error {
	// Auto-detect underlying container runtime
	containerRuntime, endpoint, err := vessel.AutoDetectRuntime()
	if err != nil {
		return err
	}
	var containerRuntimeInterface vessel.Runtime
	switch containerRuntime {
	case vesselConstants.DOCKER:
		containerRuntimeInterface = dockerRuntime.New()
	case vesselConstants.CONTAINERD:
		containerRuntimeInterface = containerdRuntime.New(endpoint)
	case vesselConstants.CRIO:
		containerRuntimeInterface = crioRuntime.New(endpoint)
	}
	if containerRuntimeInterface == nil {
		fmt.Println("Error: Could not detect container runtime")
		os.Exit(1)
	}
	err = containerRuntimeInterface.ExtractFileSystemContainer(
		containerScan.containerId, containerScan.namespace,
		containerScan.tempDir+".tar", endpoint,
	)

	if err != nil {
		return err
	}
	runCommand("mkdir", containerScan.tempDir)
	_, stdErr, retVal := runCommand("tar", "-xf", containerScan.tempDir+".tar", "-C"+containerScan.tempDir)
	if retVal != 0 {
		return errors.New(stdErr)
	}
	runCommand("rm", containerScan.tempDir+".tar")
	return nil
}

// Function to scan extracted layers of container file system for secrets file by file
// @parameters
// containerScan - Structure with details of the container  to scan
// @returns
// []output.SecretFound - List of all secrets found
// Error - Errors, if any. Otherwise, returns nil
func (containerScan *ContainerScan) scan() ([]output.SecretFound, error) {
	var isFirstSecret bool = true
	var numSecrets uint = 0

	secrets, err := ScanSecretsInDir("", containerScan.tempDir, containerScan.tempDir, &isFirstSecret, &numSecrets, nil)
	if err != nil {
		core.GetSession().Log.Error("findSecretsInContainer: %s", err)
		return nil, err
	}

	for _, secret := range secrets {
		secret.CompleteFilename = strings.Replace(secret.CompleteFilename, containerScan.tempDir, "", 1)
	}

	return secrets, nil
}

type ContainerExtractionResult struct {
	Secrets     []output.SecretFound
	ContainerId string
}

func ExtractAndScanContainer(containerId string, namespace string) (*ContainerExtractionResult, error) {
	tempDir, err := core.GetTmpDir(containerId)
	if err != nil {
		return nil, err
	}
	defer core.DeleteTmpDir(tempDir)

	containerScan := ContainerScan{containerId: containerId, tempDir: tempDir, namespace: namespace}
	err = containerScan.extractFileSystem()

	if err != nil {
		return nil, err
	}

	secrets, err := containerScan.scan()

	if err != nil {
		return nil, err
	}
	return &ContainerExtractionResult{ContainerId: containerScan.containerId, Secrets: secrets}, nil
}
