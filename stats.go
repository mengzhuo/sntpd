package sntpd

import (
	"log"
	"net"
	"net/http"

	geoip2 "github.com/oschwald/geoip2-golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type statService struct {
	//stats
	reqCounter  *prometheus.CounterVec
	offsetGauge prometheus.Gauge
	dispGauge   prometheus.Gauge
	delayGauge  prometheus.Gauge
	pollGauge   prometheus.Gauge
	jitterGauge prometheus.Gauge
	driftGauge  prometheus.Gauge
	geoDB       *geoip2.Reader
}

func newStatService(cfg *Config) (s *statService) {

	geoDB, err := geoip2.Open(cfg.Stats.GeoDB)
	if err != nil {
		log.Print(err)
		return nil
	}

	reqCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "ntp",
		Subsystem: "requests",
		Name:      "total",
		Help:      "The total number of ntp request",
	}, []string{"cc"})
	prometheus.MustRegister(reqCounter)

	offsetGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "ntp",
		Subsystem: "stat",
		Name:      "offset_sec",
		Help:      "The offset to upper peer",
	})
	prometheus.MustRegister(offsetGauge)

	dispGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "ntp",
		Subsystem: "stat",
		Name:      "dispersion_sec",
		Help:      "The dispersion of service",
	})
	prometheus.MustRegister(dispGauge)

	delayGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "ntp",
		Subsystem: "stat",
		Name:      "delay_sec",
		Help:      "The root delay of service",
	})
	prometheus.MustRegister(delayGauge)

	jitterGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "ntp",
		Subsystem: "stat",
		Name:      "jitter_sec",
		Help:      "The jitter of service",
	})
	prometheus.MustRegister(jitterGauge)

	pollGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "ntp",
		Subsystem: "stat",
		Name:      "poll_interval_sec",
		Help:      "The poll interval of ntp",
	})
	prometheus.MustRegister(pollGauge)

	driftGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "ntp",
		Subsystem: "stat",
		Name:      "drift_ppm",
		Help:      "The drift of ntp",
	})
	prometheus.MustRegister(driftGauge)

	http.Handle("/metrics", promhttp.Handler())
	log.Printf("Listen metric: %s", cfg.Stats.PromAddr)
	go http.ListenAndServe(cfg.Stats.PromAddr, nil)

	s = &statService{
		reqCounter:  reqCounter,
		offsetGauge: offsetGauge,
		pollGauge:   pollGauge,
		delayGauge:  delayGauge,
		dispGauge:   dispGauge,
		jitterGauge: jitterGauge,
		driftGauge:  driftGauge,
		geoDB:       geoDB,
	}
	return s
}

func statsIP(s *statService, raddr *net.UDPAddr) {
	country, err := s.geoDB.Country(raddr.IP)
	if err != nil {
		log.Print("stat ip err=", err)
		return
	}
	s.reqCounter.WithLabelValues(country.Country.IsoCode).Inc()
}
