package redis

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/zly-app/component/redis"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/logger"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc/resolver"

	"github.com/zly-app/grpc/discover"
	"github.com/zly-app/grpc/pkg"
	redis_registry "github.com/zly-app/grpc/registry/redis"
)

const Name = "redis"

const (
	DelRegDataThanTimeOverdue = 3600 // 如果旧数据已经过期了则删除, 单位秒
	ReDiscoverInterval        = 30   // 主动重新发现间隔时间, 单位秒
)

func init() {
	discover.AddCreator(Name, NewDiscover)
}

type RedisDiscover struct {
	creator redis.IRedisCreator
	client  redis.UniversalClient
	t       *time.Ticker

	res map[string]*RegServer
	mx  sync.Mutex
}

type RegServer struct {
	r       *discover.Resolver
	regData []*redis_registry.RegServer
	upTime  int64 // 更新时间, 秒级时间戳
}

func (s *RedisDiscover) GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	reg, ok := s.res[serverName]
	if ok {
		return reg.r, nil
	}

	regData, err := s.discoverOne(ctx, serverName)
	if err != nil {
		return nil, err
	}

	if len(regData) == 0 {
		return nil, fmt.Errorf("server %s not found router", serverName)
	}

	address := s.makeAddress(regData)
	r := discover.NewBuilderWithScheme(Name)
	r.InitialState(resolver.State{Addresses: address})
	reg = &RegServer{
		r:       r,
		regData: regData,
		upTime:  time.Now().Unix(),
	}
	s.res[serverName] = reg
	return reg.r, nil
}

func (s *RedisDiscover) Close() {
	s.creator.Close()
	if s.t != nil {
		s.t.Stop()
	}
}

func (s *RedisDiscover) start() {
	s.t = time.NewTicker(time.Second * ReDiscoverInterval)
	for range s.t.C {
		s.reDiscoverAll()
	}
}

func (s *RedisDiscover) discoverOne(ctx context.Context, serverName string) ([]*redis_registry.RegServer, error) {
	key := redis_registry.GenRegKey(serverName)
	data, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		logger.Log.Error(ctx, "Discover grpc server err",
			zap.String("RegistryType", Name),
			zap.String("serverName", serverName),
			zap.Error(err),
		)
		return nil, err
	}

	delSeq := []string{}
	nowUnix := time.Now().Unix()
	ret := make([]*redis_registry.RegServer, 0)
	for seqNo, regData := range data {
		r := redis_registry.RegServer{}
		err = sonic.UnmarshalString(regData, &r)
		if err != nil {
			logger.Log.Warn(ctx, "Discover grpc Unmarshal regData err",
				zap.String("RegistryType", Name),
				zap.String("serverName", serverName),
				zap.String("regData", regData),
				zap.Error(err),
			)
			delSeq = append(delSeq, seqNo)
			continue
		}
		// 检查过期
		if nowUnix-r.Deadline >= DelRegDataThanTimeOverdue {
			logger.Log.Warn(ctx, "Discover grpc regData is time overdue",
				zap.String("RegistryType", Name),
				zap.String("serverName", serverName),
				zap.String("regData", regData),
			)
			delSeq = append(delSeq, seqNo)
			continue
		}

		ret = append(ret, &r)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].SeqNo < ret[j].SeqNo
	})

	// 删除无效的序号
	if len(delSeq) > 0 {
		err = s.client.HDel(ctx, key, delSeq...).Err()
		if err != nil {
			logger.Log.Error(ctx, "Discover grpc del old regData err",
				zap.String("RegistryType", Name),
				zap.String("serverName", serverName),
				zap.Strings("delSeq", delSeq),
				zap.Error(err),
			)
		}
	}
	return ret, nil
}

func (s *RedisDiscover) makeAddress(regData []*redis_registry.RegServer) []resolver.Address {
	addrList := make([]resolver.Address, 0, len(regData))
	for _, a := range regData {
		addr := resolver.Address{Addr: a.Endpoint}
		addr = pkg.SetAddrInfo(addr, &pkg.AddrInfo{
			Name:     a.Name,
			Endpoint: a.Endpoint,
			Weight:   a.Weight,
		})
		addrList = append(addrList, addr)
	}
	return addrList
}

func (s *RedisDiscover) reDiscoverOne(ctx context.Context, serverName string) error {
	regData, err := s.discoverOne(ctx, serverName)
	if err != nil {
		return err
	}

	addrList := s.makeAddress(regData)

	s.mx.Lock()
	defer s.mx.Unlock()

	reg, ok := s.res[serverName]
	if !ok {
		return nil
	}

	// 对比
	needUpdate := false
	if len(regData) != len(reg.regData) {
		needUpdate = true
	} else {
		for i := range regData {
			if regData[i].SeqNo != regData[i].SeqNo {
				needUpdate = true
				break
			}
		}
	}

	if needUpdate {
		reg.r.UpdateState(resolver.State{Addresses: addrList})
		reg.regData = regData
	}
	reg.upTime = time.Now().Unix()
	return nil
}

func (s *RedisDiscover) reDiscoverAll() {
	ctx, span := utils.Otel.StartSpan(context.Background(), "ReDiscoverGrpcServer")
	defer utils.Otel.EndSpan(span)

	s.mx.Lock()
	copyRes := make(map[string]*RegServer, len(s.res))
	for k, v := range s.res {
		copyRes[k] = v
	}
	s.mx.Unlock()

	for serverName, reg := range copyRes {
		nowUnix := time.Now().Unix()
		if nowUnix-reg.upTime < ReDiscoverInterval/2 {
			continue
		}

		err := s.reDiscoverOne(ctx, serverName)
		if err != nil {
			logger.Log.Error(ctx, "ReDiscover grpc server err",
				zap.String("DiscoverType", Name),
				zap.String("serverName", serverName),
				zap.Any("reg", reg),
				zap.Error(err),
			)
			return
		}
	}
}

func NewDiscover(app core.IApp, address string) (discover.Discover, error) {
	creator := redis.NewRedisCreator(app)
	client := creator.GetRedis(address)
	rr := &RedisDiscover{
		creator: creator,
		client:  client,
		res:     make(map[string]*RegServer),
	}
	go rr.start()
	return rr, nil
}
