package sntpd

import (
	"log"
	"net"
	"time"
)

type Service struct {
	conn     *net.UDPConn
	clock    *Clock
	stat     *State
	template []byte
}

func NewService(cfg string) *Servic {

}

func (s *Service) workerDo(i int) {
	var (
		n           int
		remoteAddr  *net.UDPAddr
		err         error
		receiveTime time.Time
	)

	p := make([]byte, 48)

	defer func(i int) {
		if r := recover(); r != nil {
			log.Printf("Worker: %d falted, reason:%s, read:%d", i, r, n)
		} else {
			log.Printf("Worker: %d exited, reason:%s, read:%d", i, err, n)
		}
	}(i)

	for {
		n, remoteAddr, err = s.conn.ReadFromUDP(p)
		if err != nil {
			return
		}

		receiveTime = time.Now()
		if n < 48 {
			log.Printf("worker: %s send small packet ", remoteAddr.String())
			continue
		}

		switch p[Mode] {
		case ModeReserved:
			fallthrough
		case ModeClient:
			copy(p[0:OriginTimeStamp], s.template)
			copy(p[OriginTimeStamp:OriginTimeStamp+8],
				p[TransmitTimeStamp:TransmitTimeStamp+8])
			SetUint64(p, ReceiveTimeStamp, toNtpTime(receiveTime))
			SetUint64(p, TransmitTimeStamp, toNtpTime(time.Now()))
			_, err = s.conn.WriteToUDP(p, raddr)
			if err != nil {
				log.Printf("worker: %s write failed.", remoteAddr.String())
			}
		default:
			log.Printf("%s not client request", remoteAddr.String())
			continue
		}
	}
}
