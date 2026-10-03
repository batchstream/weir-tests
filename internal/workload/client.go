package workload

import (
	"context"
	"errors"

	weir "github.com/batchstream/weir-go"
)

// Client owns one process's connection pool, but never creates or drops data.
// Only the coordinator owns setup, reset and independent verification.
type Client struct {
	Executor Executor
	direct   *directPath
	weir     *weir.Client
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
	client := &Client{}
	if opts.Path == "direct" {
		direct, err := openDirect(ctx, opts.Dataset)
		if err != nil {
			return nil, err
		}
		client.direct, client.Executor = direct, direct
		return client, nil
	}
	open := weir.OpenOptions{Seed: opts.Dataset.Config.WeirSeed, Stores: []string{opts.Dataset.Config.StoreName}}
	sdk, err := weir.Open(ctx, open)
	if err != nil {
		return nil, err
	}
	client.weir = sdk
	path := &weirPath{dataset: opts.Dataset, client: sdk, info: opts.Evidence}
	client.Executor = path
	return client, nil
}

func (c *Client) Close() error {
	if c.direct != nil {
		return c.direct.close()
	}
	if c.weir != nil {
		return c.weir.Close()
	}
	return nil
}
