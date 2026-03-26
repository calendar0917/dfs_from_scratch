package runtime

import (
	"fmt"
	"net"
)

func MustListen(listen func(network, address string) (net.Listener, error), network, address string) (net.Listener, error) {
	lis, err := listen(network, address)
	if err != nil {
		return nil, fmt.Errorf("listen %s %s: %w", network, address, err)
	}
	return lis, nil
}
