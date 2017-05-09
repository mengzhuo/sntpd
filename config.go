package sntpd

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
