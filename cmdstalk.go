/*
	Cmdstalk is a unix-process-based [beanstalkd][beanstalkd] queue broker.

	Written in [Go][golang], cmdstalk uses the [beanstalkd/go-beanstalk][beanstalk]
	library to interact with the [beanstalkd][beanstalkd] queue daemon.

	Each job is passed as stdin to a new instance of the configured worker
	command.  On `exit(0)` the job is deleted. On `exit(1)` (or any non-zero
	status) the job is released with an exponential-backoff delay (releases^4),
	up to 10 times.

	If the worker has not finished by the time the job TTR is reached, the
	worker is killed (SIGTERM, SIGKILL) and the job is allowed to time out.
	When the job is subsequently reserved, the `timeouts: 1` will cause it to
	be buried.

	In this way, job workers can be arbitrary commands, and queue semantics are
	reduced down to basic unix concepts of exit status and signals.
*/
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/edo1/cmdstalk/broker"
	"github.com/edo1/cmdstalk/cli"
)

func main() {
	opts := cli.MustParseFlags()

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		log.Println("received interrupt. quitting.")
		cancel()
	}()

	bd := broker.NewBrokerDispatcher(ctx, opts.Address, opts.Cmd, opts.PerTube, opts.MaxJobs)

	if opts.All {
		bd.RunAllTubes()
	} else {
		bd.RunTubes(opts.Tubes)
	}

	bd.Wait()
}
