package balance

import (
	"google.golang.org/grpc"
)

func NewRoundRobinBalance() grpc.DialOption {
	return grpc.WithDefaultServiceConfig(`{ "loadBalancingConfig": [{"round_robin": {}}] }`)
}
