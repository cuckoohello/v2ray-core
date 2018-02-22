package protocol

import (
	"github.com/sourcegraph/checkup"
	"log"
	"sync"
	"time"
)

type ServerList struct {
	sync.RWMutex
	servers []*ServerSpec
	all     []*ServerSpec
}

func NewServerList() *ServerList {
	return &ServerList{}
}

func (sl *ServerList) AddServer(server *ServerSpec) {
	sl.Lock()
	defer sl.Unlock()

	sl.servers = append(sl.servers, server)
}

func (sl *ServerList) Size() uint32 {
	sl.RLock()
	defer sl.RUnlock()

	return uint32(len(sl.servers))
}

func (sl *ServerList) GetServer(idx uint32) *ServerSpec {
	sl.Lock()
	defer sl.Unlock()

	for {
		if idx >= uint32(len(sl.servers)) {
			return nil
		}

		server := sl.servers[idx]
		if !server.IsValid() {
			sl.removeServer(idx)
			continue
		}

		return server
	}
}

func (sl *ServerList) Check(timeout time.Duration, interval time.Duration) {
	sl.all = append(sl.servers, nil)
	var checkers []checkup.Checker
	for _, server := range sl.servers {
		checkers = append(checkers, checkup.TCPChecker{
			Name:    server.PickUser().Email,
			URL:     server.Destination().NetAddr(),
			Timeout: timeout,
		})
	}

	c := checkup.Checkup{
		Checkers: checkers,
		Notifier: sl,
	}
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if _, err := c.Check(); err != nil {
				log.Println(err)
			}
		}
	}()
}

func (sl *ServerList) Notify(results []checkup.Result) error {
	sl.Lock()
	defer sl.Unlock()
	sl.servers = nil
	for index, result := range results {
		if !result.Down {
			sl.servers = append(sl.servers, sl.all[index])
		}
	}
	return nil
}

func (sl *ServerList) removeServer(idx uint32) {
	n := len(sl.servers)
	sl.servers[idx] = sl.servers[n-1]
	sl.servers = sl.servers[:n-1]
}

type ServerPicker interface {
	PickServer() *ServerSpec
}

type RoundRobinServerPicker struct {
	sync.Mutex
	serverlist *ServerList
	nextIndex  uint32
}

func NewRoundRobinServerPicker(serverlist *ServerList) *RoundRobinServerPicker {
	return &RoundRobinServerPicker{
		serverlist: serverlist,
		nextIndex:  0,
	}
}

func (p *RoundRobinServerPicker) PickServer() *ServerSpec {
	p.Lock()
	defer p.Unlock()

	next := p.nextIndex
	server := p.serverlist.GetServer(next)
	if server == nil {
		next = 0
		server = p.serverlist.GetServer(0)
	}
	next++
	if next >= p.serverlist.Size() {
		next = 0
	}
	p.nextIndex = next

	return server
}
