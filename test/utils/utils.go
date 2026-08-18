package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
)

// Run executes the provided command from the project root.
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := redactCmdArgs(cmd.Args)
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed with error: (%w) %s", command, err, string(output))
	}

	return string(output), nil
}

func redactCmdArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if after, ok := strings.CutPrefix(a, "--from-literal="); ok {
			key, _, ok := strings.Cut(after, "=")
			if ok {
				out[i] = "--from-literal=" + key + "=[redacted]"
				continue
			}
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster.
// Tries `kind load docker-image` first; if the daemon uses the containerd image store
// (common on GHA), falls back to `docker save` + `kind load image-archive`.
func LoadImageToKindClusterWithName(name string) error {
	cluster := "kind"
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	cmd := exec.Command("kind", "load", "docker-image", name, "--name", cluster)
	if _, err := Run(cmd); err == nil {
		return nil
	}

	_, _ = fmt.Fprintf(GinkgoWriter, "kind load docker-image failed; falling back to image-archive\n")
	tar, err := os.CreateTemp("", "kind-image-*.tar")
	if err != nil {
		return fmt.Errorf("create temp image archive: %w", err)
	}
	tarPath := tar.Name()
	_ = tar.Close()
	defer os.Remove(tarPath)

	save := exec.Command("docker", "save", "-o", tarPath, name)
	if _, err := Run(save); err != nil {
		return err
	}
	load := exec.Command("kind", "load", "image-archive", tarPath, "--name", cluster)
	_, err = Run(load)
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.SplitSeq(output, "\n")
	for element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}
