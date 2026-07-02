package runtimemanager

import (
	"fmt"
	"net"
	"sync"
)

type portAllocator struct {
	start int
	end   int
	used  map[int]bool
	mutex sync.Mutex
}

func newPortAllocator(start, end int) *portAllocator {
	return &portAllocator{
		start: start,
		end:   end,
		used:  make(map[int]bool),
	}
}

func (pa *portAllocator) Allocate() (int, error) {
	pa.mutex.Lock()
	defer pa.mutex.Unlock()

	for port := pa.start; port <= pa.end; port++ {
		if !pa.used[port] && isPortAvailable(port) {
			pa.used[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports")
}

func (pa *portAllocator) Release(port int) {
	pa.mutex.Lock()
	defer pa.mutex.Unlock()
	delete(pa.used, port)
}

func isPortAvailable(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
