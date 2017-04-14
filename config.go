package sntpd

type Config struct {
	Worker       int      `yaml:"worker"`
	ListenAddr   string   `yaml:"listen_addr"`
	DriftFile    string   `yaml:"drift_file"`
	StratumPool  []string `yaml:"stratum_pool,flow"`
	Restrict     []string `yaml:"restrict"`
	MinPoll      int8     `yaml:"minpoll"`
	MaxPoll      int8     `yaml:"maxpoll"`
	RtcLocalTime bool     `yaml:"rtclocal"`
	Stats        *Stats   `yaml:"stats"`
}

type Stats struct {
	PromAddr string `yaml:"pro_addr"`
	GeoDB    string `yaml:"geodb"`
}
