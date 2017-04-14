package sntpd

import (
	"io/ioutil"
	"log"
	"math"
	"net"
	"runtime"
	"time"

	yaml "gopkg.in/yaml.v2"
)

const (
	ClockMinStep float64 = 300 /* default stepout threshold (s) */
	NTPAccuracy          = 128 * time.Millisecond
)

// We read service is much more than write, so we keep msg needed format
type Service struct {
	cfgPath string
	cfg     *Config
	conn    *net.UDPConn
	peers   []*Peer
	drift   float64

	offset     time.Duration
	delay      time.Duration
	dispersion time.Duration
	jitter     time.Duration
	referTime  time.Time
	leap       uint8
	stratum    uint8
	poll       int8
	precision  int8

	delayDispData [8]byte
	responseTmpl  []byte
	stats         *statService
}

func absTime(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func (s *Service) setFromPeer(p *Peer) {
	SetLi(s.responseTmpl, s.leap)
	SetVersion(s.responseTmpl, 4)
	SetMode(s.responseTmpl, ModeServer)

	s.stratum = p.stratum + 1
	SetUint8(s.responseTmpl, Stratum, s.stratum)

	SetInt8(s.responseTmpl, Poll, s.poll)
	SetInt8(s.responseTmpl, ClockPrecision, s.precision)

	s.delay = p.rootDelay + p.delay
	SetUint32(s.responseTmpl, RootDelayPos, toNtpShortTime(s.delay))
	s.stats.delayGauge.Set(s.delay.Seconds())

	absof := absTime(s.offset)
	s.dispersion = p.rootDisp + p.disp + s.jitter + secondToDuration(Phi*time.Now().Sub(p.epoch).Seconds()) + absof
	s.stats.offsetGauge.Set(s.offset.Seconds())
	s.stats.dispGauge.Set(s.dispersion.Seconds())
	s.stats.jitterGauge.Set(s.jitter.Seconds())
	s.stats.driftGauge.Set(s.drift)

	SetUint32(s.responseTmpl, RootDispersionPos, toNtpShortTime(s.dispersion))
	s.referTime = p.epoch

	SetUint64(s.responseTmpl, ReferenceTimeStamp, toNtpTime(s.referTime))
	log.Printf("reftime=%s", s.referTime)
	SetUint32(s.responseTmpl, ReferIDPos, p.refid)

	if absof > NTPAccuracy {
		s.poll = s.cfg.MinPoll
		return
	}

	switch absof / (NTPAccuracy / 10) {
	case 0: // < 12.8ms
		s.poll += 1
	case 1:
	default:
		s.poll -= 1
	}

	if s.poll > s.cfg.MaxPoll {
		s.poll = s.cfg.MaxPoll
	}

	if s.poll < s.cfg.MinPoll {
		s.poll = s.cfg.MinPoll
	}
}

func NewService(cfgPath string) *Service {

	return &Service{
		cfgPath:      cfgPath,
		referTime:    time.Now(),
		precision:    systemPrecision(),
		responseTmpl: make([]byte, 48, 48),
	}
}

func DefaultConfig() *Config {

	return &Config{
		StratumPool: []string{},
		ListenAddr:  ":123",
		DriftFile:   "sntpd.drift",
		Worker:      runtime.NumCPU(),
		MinPoll:     4,
		MaxPoll:     10,
	}
}

func (s *Service) ReloadConfig() (err error) {
	data, err := ioutil.ReadFile(s.cfgPath)
	if err != nil {
		return err
	}

	d := DefaultConfig()
	log.Print(s.cfgPath, string(data))
	err = yaml.Unmarshal(data, d)

	if err == nil {
		s.cfg = d
	}

	if s.cfg.Worker < 1 {
		s.cfg.Worker = 1
	}

	if s.cfg.MinPoll < 4 {
		s.cfg.MinPoll = 4
	}
	if s.cfg.MaxPoll > 17 {
		s.cfg.MaxPoll = 17
	}
	if s.cfg.MinPoll > s.cfg.MaxPoll {
		s.cfg.MinPoll = s.cfg.MaxPoll
	}

	s.peers = make([]*Peer, len(s.cfg.StratumPool))

	for i, k := range s.cfg.StratumPool {
		s.peers[i] = &Peer{Addr: k}
		s.peers[i].init()
	}

	s.poll = s.cfg.MinPoll
	return
}

func (s *Service) ListenAndServe() (err error) {
	err = s.ReloadConfig()
	if err != nil {
		return err
	}

	log.Print(s.cfg)

	s.Stop()

	err = s.Listen(s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	s.initClock()
	s.stats = newStatService(s.cfg)

	go func() {
		time.Sleep(time.Hour)
		log.Printf("write drift:%f", s.drift)
		s.WriteDrift()
	}()

	for i := 0; i < 3; i++ {
		log.Printf("init polling...%d", i)
		start := time.Now()
		s.sample()
		interval := 4*time.Second - time.Now().Sub(start)
		if interval.Seconds() > 0 {
			time.Sleep(interval)
		}
	}
	s.sample()
	s.doPoll()
	s.poll = s.cfg.MinPoll

	for i := 0; i < s.cfg.Worker; i++ {
		go s.makeWorker(i)
	}
	go s.doSample()
	var intervalSec time.Duration
	for {
		intervalSec = pollToDuration(s.poll)
		s.stats.pollGauge.Set(intervalSec.Seconds())
		log.Printf("next poll interval=%s", intervalSec)
		time.Sleep(intervalSec)
		s.doPoll()
	}
}

func (s *Service) doSample() {

	var intervalSec time.Duration
	for {
		switch {
		case s.poll < 6:
			intervalSec = pollToDuration(s.poll)
		case s.poll < 9:
			intervalSec = pollToDuration(s.poll) / 2
		default:
			intervalSec = pollToDuration(s.poll) / 4
		}
		time.Sleep(intervalSec - 5100*time.Millisecond)
		s.sample()
	}
}

func pollToDuration(poll int8) time.Duration {
	return time.Duration(math.Pow(2, float64(poll)) * float64(time.Second))
}

func (s *Service) initClock() {
	err := s.LoadDrift()
	log.Printf("load drift file:%f err=%v", s.drift, err)
	initClock(int64(s.drift * 65536))
}

func (s *Service) serveClient(raddr *net.UDPAddr, receiveTime time.Time, p []byte) (err error) {

	copy(p[0:OriginTimeStamp], s.responseTmpl)
	copy(p[OriginTimeStamp:OriginTimeStamp+8],
		p[TransmitTimeStamp:TransmitTimeStamp+8])

	SetUint64(p, ReceiveTimeStamp, toNtpTime(receiveTime))
	SetUint64(p, TransmitTimeStamp, toNtpTime(time.Now()))
	_, err = s.conn.WriteToUDP(p, raddr)
	return

}
func (s *Service) serveSymmetricActive(raddr *net.UDPAddr, receiveTime time.Time, p []byte) (err error) {
	SetMode(p, ModeSymmetricPassive)
	SetUint8(p, Stratum, s.stratum)
	SetInt8(p, Poll, s.poll)
	SetUint8(p, ClockPrecision, 0xea)

	copy(p[OriginTimeStamp:OriginTimeStamp+8], p[TransmitTimeStamp:TransmitTimeStamp+8])

	SetUint64(p, ReferenceTimeStamp, toNtpTime(s.referTime))
	SetUint64(p, ReceiveTimeStamp, toNtpTime(receiveTime))
	SetUint64(p, TransmitTimeStamp, toNtpTime(time.Now()))
	_, err = s.conn.WriteToUDP(p, raddr)
	return
}

func (s *Service) makeWorker(i int) {

	p := make([]byte, 48, 64)
	var (
		raddr       *net.UDPAddr
		err         error
		n           int
		receiveTime time.Time
	)

	log.Printf("worker %d start runing", i)

	for {
		n, raddr, err = s.conn.ReadFromUDP(p)

		if err != nil {
			log.Printf("Worker %d stopped, reason:%s, read:%d", i, err, n)
			return
		}
		receiveTime = time.Now()
		if n < 48 {
			log.Printf("%s send small packet ", raddr.String())
			continue
		}

		switch GetMode(p) {
		case ModeClient:
			err = s.serveClient(raddr, receiveTime, p)
		case ModeReserved:
			err = s.serveClient(raddr, receiveTime, p)
		case ModeSymmetricActive:
			err = s.serveSymmetricActive(raddr, receiveTime, p)
		default:
			log.Printf("%s mode %d is not supported", raddr.String(), GetMode(p))
			continue
		}

		if err != nil {
			log.Print(err)
			continue
		}
		err = nil
		statsIP(s.stats, raddr)
	}

}

func (s *Service) Stop() {
	if s.conn == nil {
		return
	}

	if err := s.conn.Close(); err != nil {
		log.Print(err)
	}
}

func (s *Service) Listen(addr string) (err error) {

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}

	s.conn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	return
}
