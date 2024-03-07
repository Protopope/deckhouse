package registry

type ClientConfig struct {
	Address string `json:"address" yaml:"address"`
	Path    string `json:"path" yaml:"path"`
	Scheme  string `json:"scheme" yaml:"scheme"`
	CA      string `json:"ca,omitempty" yaml:"ca,omitempty"`
	Auth    string `json:"auth" yaml:"auth"`
}
