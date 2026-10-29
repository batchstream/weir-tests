package fixture

import "runtime"

// ResourceTarget exposes only the identities of resources owned by this lab,
// or of an explicitly supplied remote deployment. Nothing is discovered or adopted.
type ResourceTarget struct {
	Container     string
	DockerHost    string
	DockerConfig  string
	ProcessID     int
	WeirProcessID int
	AllocatedCPUs float64
	Diagnostics   string
	// Remote samples an explicitly supplied deployment: database CPU from the
	// database's own API, Weir CPU from its diagnostics, which may be non-loopback.
	Remote bool
}

func (c *Cluster) Resources(backend string) ResourceTarget {
	c.mu.Lock()
	defer c.mu.Unlock()
	target := ResourceTarget{AllocatedCPUs: c.options.DatabaseCPUs, DockerHost: c.dockerHost, DockerConfig: c.Directory + "/docker-config"}
	if len(c.Nodes) > 0 {
		target.Diagnostics = c.Nodes[0].Diagnostics
	}
	if len(c.processes) > 0 {
		target.WeirProcessID = c.processes[0].cmd.Process.Pid
	}
	if backend == "mongo" && c.nativeMongo != nil {
		target.ProcessID = c.nativeMongo.cmd.Process.Pid
		target.AllocatedCPUs = float64(runtime.NumCPU())
		return target
	}
	for _, owned := range c.containers {
		if owned.Backend == backend && !owned.removed {
			target.Container = owned.ID
			break
		}
	}
	return target
}
