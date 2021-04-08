package main

import (
	"github.com/yylt/rtspmux/config"
	"log"
	"os"
	"os/signal"
)


func main()  {

	conf:=config.ConfigRead()

	srv := NewServer(conf)

	go func() {
		if err := srv.StartServer(); err != nil {
			log.Println(err)
		}
	}()

	c := make(chan os.Signal, 1)

	signal.Notify(c, os.Interrupt)

	<-c

	srv.Stop()

	log.Println("shutting down")
	os.Exit(0)

}
