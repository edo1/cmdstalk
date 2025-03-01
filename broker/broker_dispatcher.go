package broker

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/beanstalkd/go-beanstalk"
)

const (
	// ListTubeDelay is the time between sending list-tube to beanstalkd
	// to discover and watch newly created tubes.
	ListTubeDelay = 10 * time.Second
)

// BrokerDispatcher manages the running of Broker instances for tubes.  It can
// be manually told tubes to start, or it can poll for tubes as they are
// created. The `perTube` option determines how many brokers are started for
// each tube.
type BrokerDispatcher struct {
	address     string
	cmd         string
	conn        *beanstalk.Conn
	perTube     uint64
	tubeSet     map[string]bool
	jobReceived chan<- struct{}
	ctx         context.Context
	wg          sync.WaitGroup
}

func NewBrokerDispatcher(parentCtx context.Context, address, cmd string, perTube, maxJobs uint64) *BrokerDispatcher {
	ctx, cancel := context.WithCancel(parentCtx)
	jobReceived := make(chan struct{})
	go limittedCountGenerator(maxJobs, cancel, jobReceived)
	return &BrokerDispatcher{
		address:     address,
		cmd:         cmd,
		perTube:     perTube,
		tubeSet:     make(map[string]bool),
		jobReceived: jobReceived,
		ctx:         ctx,
	}
}

// RunTube runs broker(s) for the specified tube.
// The number of brokers started is determined by the perTube argument to
// NewBrokerDispatcher.
func (bd *BrokerDispatcher) RunTube(tube string) {
	bd.tubeSet[tube] = true
	for i := uint64(0); i < bd.perTube; i++ {
		bd.runBroker(tube, i)
	}
}

// RunTube runs brokers for the specified tubes.
func (bd *BrokerDispatcher) RunTubes(tubes []string) {
	for _, tube := range tubes {
		bd.RunTube(tube)
	}
}

// RunAllTubes polls beanstalkd, running broker as new tubes are created.
func (bd *BrokerDispatcher) RunAllTubes() (err error) {
	conn, err := beanstalk.Dial("tcp", bd.address)
	if err == nil {
		bd.conn = conn
	} else {
		return
	}

	go func() {
		ticker := instantTicker(ListTubeDelay)
		for _ = range ticker {
			if e := bd.watchNewTubes(); e != nil {
				log.Println(e)
			}
		}
	}()

	return
}

// limittedCountGenerator creates a channel that returns a boolean channel with
// nlimit true's and false otherwise. If nlimit is 0 it the channel will always
// be containing true.
func limittedCountGenerator(nlimit uint64, cancel context.CancelFunc, eventHappened <-chan struct{}) {
	ngenerated := uint64(1)
	for range eventHappened {
		if nlimit != 0 && ngenerated == nlimit {
			log.Println("reached job limit. quitting.")
			cancel()
		}
		ngenerated++
	}
}

func (bd *BrokerDispatcher) runBroker(tube string, slot uint64) {
	bd.wg.Add(1)
	go func() {
		defer bd.wg.Done()
		b := New(bd.ctx, bd.address, tube, slot, bd.cmd, nil, bd.jobReceived)
		b.Run(nil)
	}()
}

func (bd *BrokerDispatcher) Wait() {
	bd.wg.Wait()
}

func (bd *BrokerDispatcher) watchNewTubes() (err error) {
	tubes, err := bd.conn.ListTubes()
	if err != nil {
		return
	}

	for _, tube := range tubes {
		if !bd.tubeSet[tube] {
			bd.RunTube(tube)
		}
	}

	return
}

// Like time.Tick() but also fires immediately.
func instantTicker(t time.Duration) <-chan time.Time {
	c := make(chan time.Time)
	ticker := time.NewTicker(t)
	go func() {
		c <- time.Now()
		for t := range ticker.C {
			c <- t
		}
	}()
	return c
}
