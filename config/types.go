package config

type CacheConfig struct {
	TTLHours int `yaml:"ttl_hours"`
}

type DetectionConfig struct {
	BorderInputs        int     `yaml:"border_inputs"`
	DiscoveryCandidates int     `yaml:"discovery_candidates"`
	QueriesPerInput     int     `yaml:"queries_per_input"`
	TVThreshold         float64 `yaml:"tv_threshold"`
}

type ConcurrencyConfig struct {
	MaxWorkersPerChannel int     `yaml:"max_workers_per_channel"`
	RateLimitRPS         float64 `yaml:"rate_limit_rps"`
	TimeoutSeconds       int     `yaml:"timeout_seconds"`
	MaxRetries           int     `yaml:"max_retries"`
}

type OutputConfig struct {
	ReportDir string `yaml:"report_dir"`
}

type Config struct {
	Cache       CacheConfig       `yaml:"cache"`
	Detection   DetectionConfig   `yaml:"detection"`
	Concurrency ConcurrencyConfig `yaml:"concurrency"`
	Output      OutputConfig      `yaml:"output"`
}

type Endpoint struct {
	Name      string `yaml:"name"`
	URL       string `yaml:"url"`
	Key       string `yaml:"key"`
	Provider  string `yaml:"provider,omitempty"`
	Extrahack bool   `yaml:"extrahack,omitempty"`
}

type ModelConfig struct {
	Model    string     `yaml:"model"`
	Official Endpoint   `yaml:"official"`
	Channels []Endpoint `yaml:"channels"`
}
