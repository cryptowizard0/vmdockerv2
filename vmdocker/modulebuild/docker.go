package modulebuild

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type localImage struct {
	Name string
	ID   string
}

var progressWriter io.Writer = os.Stdout

func dockerBuild(ctx context.Context, dockerfile, contextRef, tag string, buildArgs map[string]string) error {
	cliBin, err := dockerBinary()
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "vmdocker-module-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	dockerfilePath := tmpDir + string(os.PathSeparator) + "Dockerfile"
	if err := os.WriteFile(dockerfilePath, []byte(dockerfile), 0o600); err != nil {
		return err
	}

	args := []string{"build", "--progress=plain", "-f", dockerfilePath, "-t", tag}
	for _, buildArg := range sortedBuildArgs(buildArgs) {
		args = append(args, "--build-arg", buildArg)
	}
	for _, proxyKey := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"} {
		if val := os.Getenv(proxyKey); val != "" {
			val = strings.ReplaceAll(val, "127.0.0.1", "host.docker.internal")
			val = strings.ReplaceAll(val, "localhost", "host.docker.internal")
			args = append(args, "--build-arg", proxyKey+"="+val)
		}
	}
	args = append(args, contextRef)

	cmd := exec.CommandContext(ctx, cliBin, args...)
	cmd.Stdout = progressWriter
	cmd.Stderr = progressWriter
	logProgressf("docker build started: tag=%s", tag)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed for %s: %w", tag, err)
	}
	logProgressf("docker build completed: tag=%s", tag)
	return nil
}

func dockerPull(ctx context.Context, imageName string) error {
	cliBin, err := dockerBinary()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, cliBin, "pull", imageName)
	cmd.Stdout = progressWriter
	cmd.Stderr = progressWriter
	logProgressf("docker pull started: image=%s", imageName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker pull failed for %s: %w", imageName, err)
	}
	logProgressf("docker pull completed: image=%s", imageName)
	return nil
}

func inspectImageID(ctx context.Context, imageName string) (string, error) {
	cliBin, err := dockerBinary()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, cliBin, "image", "inspect", "--format", "{{.Id}}", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect image %s failed: %w\n%s", imageName, err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func exportImageArchive(ctx context.Context, imageName string) ([]byte, error) {
	cliBin, err := dockerBinary()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, cliBin, "save", imageName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	progressReader := newArchiveProgressReader(stdout, imageName)
	if _, err := io.Copy(gz, progressReader); err != nil {
		_ = gz.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("stream docker save output for %s failed: %w", imageName, err)
	}
	if err := gz.Close(); err != nil {
		_ = cmd.Wait()
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("docker save failed for %s: %w\n%s", imageName, err, stderr.String())
	}
	logProgressf("docker save completed: image=%s raw_size=%s compressed_size=%s", imageName, formatBytes(progressReader.total), formatBytes(int64(archive.Len())))
	return archive.Bytes(), nil
}

func dockerBinary() (string, error) {
	cliBin, err := exec.LookPath("docker")
	if err != nil {
		return "", fmt.Errorf("docker CLI is not available: %w", err)
	}
	return cliBin, nil
}

func logProgressf(format string, args ...any) {
	if progressWriter == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(progressWriter, "[module] %s %s\n", time.Now().Format("15:04:05"), msg)
}

type archiveProgressReader struct {
	reader     io.Reader
	imageName  string
	total      int64
	nextReport int64
}

func newArchiveProgressReader(reader io.Reader, imageName string) *archiveProgressReader {
	const reportEvery = 128 << 20
	return &archiveProgressReader{
		reader:     reader,
		imageName:  imageName,
		nextReport: reportEvery,
	}
}

func (r *archiveProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.total += int64(n)
		for r.total >= r.nextReport {
			logProgressf("docker save streaming: image=%s raw_size=%s", r.imageName, formatBytes(r.total))
			r.nextReport += 128 << 20
		}
	}
	return n, err
}

func sortedBuildArgs(args map[string]string) []string {
	if len(args) == 0 {
		return nil
	}
	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buildArgs := make([]string, 0, len(keys))
	for _, key := range keys {
		buildArgs = append(buildArgs, key+"="+args[key])
	}
	return buildArgs
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
