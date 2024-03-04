package util

import (
	"bytes"
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type ExecResult struct {
	StdOut   string
	StdErr   string
	ExitCode int
}

func ExecSync(client *client.Client, containerID string, config types.ExecConfig, outputWriter *io.Writer) (*ExecResult, error) {
	ctx := context.Background()

	config.AttachStderr = true
	config.AttachStdout = true
	config.AttachStdin = false

	config.Env = append(config.Env, "DEBIAN_FRONTEND=noninteractive")

	response, err := client.ContainerExecCreate(ctx, containerID, config)
	if err != nil {
		return nil, err
	}

	result, err := InspectExecResp(client, ctx, response.ID, outputWriter)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func InspectExecResp(client *client.Client, ctx context.Context, id string, outputWriter *io.Writer) (*ExecResult, error) {
	resp, err := client.ContainerExecAttach(ctx, id, types.ExecStartCheck{})
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	// read the output
	outputDone := make(chan error)

	outBuf := bytes.Buffer{}
	errBuf := bytes.Buffer{}

	go func() {
		// StdCopy demultiplexes the stream into two buffers
		outMw := io.Writer(&outBuf)
		errMw := io.Writer(&errBuf)

		if outputWriter != nil {
			outMw = io.MultiWriter(&outBuf, *outputWriter)
			errMw = io.MultiWriter(&errBuf, *outputWriter)
		}

		_, err = stdcopy.StdCopy(outMw, errMw, resp.Reader)
		outputDone <- err
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return nil, err
		}
		break

	case <-ctx.Done():
		return nil, ctx.Err()
	}

	stdout, err := io.ReadAll(&outBuf)
	if err != nil {
		return nil, err
	}
	stderr, err := io.ReadAll(&errBuf)
	if err != nil {
		return nil, err
	}

	res, err := client.ContainerExecInspect(ctx, id)
	if err != nil {
		return nil, err
	}

	return &ExecResult{
		ExitCode: res.ExitCode,
		StdOut:   string(stdout),
		StdErr:   string(stderr),
	}, nil
}