package agent

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	APIURL     string   `yaml:"api_url"`
	AgentToken string   `yaml:"agent_token"`
	NodeID     string   `yaml:"node_id"`
	Tags       []string `yaml:"tags"`
}

func Load(path string) (*Config, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(bs, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// GetNodeID returns the node ID.
func (c *Config) GetNodeID() string {
	return c.NodeID
}

// GetTags returns the tags.
func (c *Config) GetTags() []string {
	return c.Tags
}
