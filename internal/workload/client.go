package workload

import (
	"context"
	"errors"
	"io"

	weir "github.com/batchstream/weir-go"
)

// Client owns one process's connection pool, but never creates or drops data.
// Only the coordinator owns setup, reset and independent verification.
type Client struct {
	Executor Executor
	io.Closer
}

type ClientOptions struct {
	Dataset  *Dataset
	Path     string
	Evidence Evidence
}

func OpenClient(ctx context.Context, opts ClientOptions) (*Client, error) {
	if opts.Dataset == nil || (opts.Path != "direct" && opts.Path != "weir") {
		return nil, errors.New("client requires a dataset and direct or weir path")
	}
	if opts.Path == "direct" {
		direct, err := openDirect(ctx, opts.Dataset)
		if err != nil {
			return nil, err
		}
		client := &Client{Executor: direct, Closer: direct}
		return client, nil
	}
	open := weir.OpenOptions{Seed: opts.Dataset.Config.WeirSeed, Stores: []string{opts.Dataset.Config.StoreName}}
	sdk, err := weir.Open(ctx, open)
	if err != nil {
		return nil, err
	}
	path := &weirPath{dataset: opts.Dataset, client: sdk, info: opts.Evidence}
	client := &Client{Executor: path, Closer: sdk}
	return client, nil
}
