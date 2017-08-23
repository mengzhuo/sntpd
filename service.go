package sntpd

import (
	"log"
	"net"
	"sync"
	"time"
)

type Service struct {
	conn     *net.UDPConn
	clock    *Clock
	stat     *State
	cfg      *Config
	template []byte
}

func NewService(cfgPath string) (s *Service, err error) {
	cfg, err := newConfig(cfgPath)
	if err != nil {
		return
	}
	s = &Service{
		cfg: cfg,
	}
	return
}

func (s *Service) ListenAndServe() (err error) {
	var (
		addr *net.UDPAddr
	)
	addr, err = net.ResolveUDPAddr("udp", s.cfg.Listen)
	if err != nil {
		return
	}

	s.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return
	}

	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Worker; i++ {
		go s.workerDo(i, &wg)
	}
	wg.Wait()
	err = s.conn.Close()
	log.Print("Exit")
	return
}

func (s *Service) workerDo(i int, wg *sync.WaitGroup) {
	var (
		n           int
		remoteAddr  *net.UDPAddr
		err         error
		receiveTime time.Time
	)
	wg.Add(1)

	p := make([]byte, 48)

	defer func(i int) {
		if r := recover(); r != nil {
			log.Printf("Worker: %d fatal, reason:%s, read:%d", i, r, n)
		} else {
			log.Printf("Worker: %d exited, reason:%s, read:%d", i, err, n)
		}
		wg.Done()
	}(i)

	for {
		n, remoteAddr, err = s.conn.ReadFromUDP(p)
		if err != nil {
			return
		}

		receiveTime = time.Now()
		if n < 48 {
			log.Printf("worker: %s get small packet %d", remoteAddr.String(), n)
			continue
		}

		// GetMode
		switch p[LiVnMode] &^ 0xf8 {
		case ModeReserved:
			fallthrough
		case ModeClient:
			copy(p[0:OriginTimeStamp], s.template)
			copy(p[OriginTimeStamp:OriginTimeStamp+8],
				p[TransmitTimeStamp:TransmitTimeStamp+8])
			SetUint64(p, ReceiveTimeStamp, toNtpTime(receiveTime))
			SetUint64(p, TransmitTimeStamp, toNtpTime(time.Now()))
			_, err = s.conn.WriteToUDP(p, remoteAddr)
			if err != nil {
				log.Printf("worker: %s write failed. %s", remoteAddr.String(), err)
			}
		default:
			log.Printf("%s not client request", remoteAddr.String())
		}
	}
}
