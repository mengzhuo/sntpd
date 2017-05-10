package sntpd

import (
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	Listen     string
	StatListen string
	GeoDB      string

	ClockList []string
	Restrict  []string

	Worker  int
	MinPoll int8
	MaxPoll int8
}

func newConfig(p string) (cfg *Config, err error) {
	var b []byte
	b, err = ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	cfg = &Config{}
	err = yaml.Unmarshal(b, cfg)
	return
}
