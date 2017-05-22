package main

import (
	"flag"
	"log"

	"github.com/mengzhuo/sntpd"
)

var (
	cfgPath = flag.String("c", "config.yml", "Default config path")
)

func main() {
	flag.Parse()
	service, err := sntpd.NewService(*cfgPath)
	if err != nil {
		log.Print(err)
	}
	service.ListenAndServe()
}
