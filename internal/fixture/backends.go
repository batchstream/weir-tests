package fixture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type mongoStartup struct {
	Container *container
	Process   *process
}

func (c *Cluster) readyMongo(ctx context.Context, startup mongoStartup) error {
	readyCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	clientOptions := options.Client().ApplyURI(c.MongoURI).SetServerSelectionTimeout(2 * time.Second).SetConnectTimeout(2 * time.Second)
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return err
	}
	defer func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = client.Disconnect(disconnectCtx)
	}()
	var last error
	for {
		if err := c.checkMongoStartup(readyCtx, startup); err != nil {
			return err
		}
		if startup.Process != nil {
			announced, err := mongoAnnounced(startup.Process.logPath, c.MongoURI)
			if err != nil {
				return err
			}
			if !announced {
				if err := pause(readyCtx); err != nil {
					return err
				}
				continue
			}
		}
		commandCtx, commandCancel := context.WithTimeout(readyCtx, 3*time.Second)
		command := bson.D{{Key: "buildInfo", Value: 1}}
		var build struct {
			Version string `bson:"version"`
		}
		err := client.Database("admin").RunCommand(commandCtx, command).Decode(&build)
		commandCancel()
		if err == nil {
			if build.Version != MongoVersion {
				return fmt.Errorf("MongoDB version %q differs from pinned %s", build.Version, MongoVersion)
			}
			break
		}
		last = err
		if err := pause(readyCtx); err != nil {
			return errors.Join(err, last)
		}
	}
	member := "127.0.0.1:27017"
	if startup.Process != nil {
		address, err := url.Parse(c.MongoURI)
		if err != nil {
			return err
		}
		member = address.Host
	}
	configuration := bson.D{{Key: "_id", Value: "weir_tests"}, {Key: "members", Value: bson.A{bson.D{{Key: "_id", Value: 0}, {Key: "host", Value: member}}}}}
	initiate := bson.D{{Key: "replSetInitiate", Value: configuration}}
	initCtx, initCancel := context.WithTimeout(readyCtx, 10*time.Second)
	err = client.Database("admin").RunCommand(initCtx, initiate).Err()
	initCancel()
	if err != nil {
		return err
	}
	for {
		if err := c.checkMongoStartup(readyCtx, startup); err != nil {
			return err
		}
		commandCtx, commandCancel := context.WithTimeout(readyCtx, 3*time.Second)
		command := bson.D{{Key: "hello", Value: 1}}
		var hello struct {
			IsWritablePrimary bool `bson:"isWritablePrimary"`
		}
		err := client.Database("admin").RunCommand(commandCtx, command).Decode(&hello)
		commandCancel()
		if err == nil && hello.IsWritablePrimary {
			break
		}
		last = err
		if err := pause(readyCtx); err != nil {
			return errors.Join(err, last, errors.New("MongoDB did not elect a writable primary"))
		}
	}
	return nil
}

func (c *Cluster) checkMongoStartup(ctx context.Context, startup mongoStartup) error {
	if startup.Process != nil {
		select {
		case <-startup.Process.done:
			return fmt.Errorf("native MongoDB exited during startup; logs: %s: %w", startup.Process.logPath, startup.Process.err)
		default:
			return nil
		}
	}
	state, err := c.inspectContainer(ctx, startup.Container.ID)
	if err != nil {
		return err
	}
	if err := verifyOwned(state, *startup.Container, c.owner); err != nil {
		return err
	}
	if !state.State.Running {
		output, logErr := c.docker(ctx, "logs", state.ID)
		return errors.Join(fmt.Errorf("MongoDB container exited during startup (exit %d, %s): %s", state.State.ExitCode, state.State.Error, strings.TrimSpace(string(output))), logErr)
	}
	return nil
}

func (c *Cluster) readySearch(ctx context.Context) error {
	readyCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	client := loopbackHTTPClient()
	defer client.CloseIdleConnections()
	var last error
	for {
		var result struct {
			Version struct {
				Number string `json:"number"`
			} `json:"version"`
		}
		versionRequest := searchRequest{Method: http.MethodGet, URL: c.SearchURL + "/", Result: &result}
		err := searchJSON(readyCtx, client, versionRequest)
		if err == nil {
			if result.Version.Number != SearchVersion {
				return fmt.Errorf("Elasticsearch version %q differs from pinned %s", result.Version.Number, SearchVersion)
			}
			break
		}
		last = err
		if err := pause(readyCtx); err != nil {
			return errors.Join(err, last)
		}
	}
	var health struct {
		TimedOut bool   `json:"timed_out"`
		Status   string `json:"status"`
		Nodes    int    `json:"number_of_nodes"`
	}
	healthRequest := searchRequest{Method: http.MethodGet, URL: c.SearchURL + "/_cluster/health?wait_for_status=yellow&timeout=2s", Result: &health}
	if err := searchJSON(readyCtx, client, healthRequest); err != nil {
		return err
	}
	if health.TimedOut || health.Nodes != 1 || health.Status != "green" && health.Status != "yellow" {
		return errors.New("Elasticsearch singleton did not become healthy")
	}
	var acknowledged struct {
		Acknowledged bool `json:"acknowledged"`
	}
	template := `{"index_patterns":["*"],"priority":1000,"template":{"settings":{"number_of_shards":1,"number_of_replicas":0}}}`
	templateRequest := searchRequest{Method: http.MethodPut, URL: c.SearchURL + "/_index_template/weir-tests", Body: template, Result: &acknowledged}
	if err := searchJSON(readyCtx, client, templateRequest); err != nil {
		return err
	}
	if !acknowledged.Acknowledged {
		return errors.New("Elasticsearch fixture index template was not acknowledged")
	}
	return nil
}

type searchRequest struct {
	Method string
	URL    string
	Body   string
	Result any
}

func searchJSON(ctx context.Context, client *http.Client, options searchRequest) error {
	request, err := http.NewRequestWithContext(ctx, options.Method, options.URL, strings.NewReader(options.Body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 2<<20 {
		return errors.New("Elasticsearch fixture response exceeds 2MiB")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Elasticsearch %s %s returned %d: %s", options.Method, options.URL, response.StatusCode, data)
	}
	return json.Unmarshal(data, options.Result)
}
