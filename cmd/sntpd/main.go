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
	service := sntpd.NewService(*cfgPath)
	log.Print(service.ListenAndServe())
}
