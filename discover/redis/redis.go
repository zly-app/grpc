package redis

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/zly-app/component/redis"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/logger"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"
	"google.golang.org/grpc/resolver"

	rredis "github.com/redis/go-redis/v9"

	"github.com/zly-app/grpc/discover"
	"github.com/zly-app/grpc/pkg"
	redis_registry "github.com/zly-app/grpc/registry/redis"
)

const Type = "redis"

const (
	DelRegDataThanTimeOverdue = 3600 // 如果旧数据已经过期了则删除, 单位秒
	ReDiscoverInterval        = 30   // 主动重新发现间隔时间, 单位秒
)

func init() {
	discover.AddCreator(Type, NewDiscover)
}

type RedisDiscover struct {
	client redis.UniversalClient
	sub    *rredis.PubSub

	t   *time.Ticker
	res map[string]*RegServer
	mx  sync.Mutex
}

type RegServer struct {
	r       *discover.Resolver
	regData []*redis_registry.RegServer
	upTime  int64 // 更新时间, 秒级时间戳
	mx      sync.Mutex
}

func (r *RegServer) Remove(seqNo int) {
	r.mx.Lock()
	defer r.mx.Unlock()

	regData := make([]*redis_registry.RegServer, 0, len(r.regData))
	for i := range r.regData {
		if r.regData[i].SeqNo != seqNo {
			regData = append(regData, r.regData[i])
		}
	}
	if len(regData) == len(r.regData) { // 没有变化/没有找到要移除的
		return
	}
	r.update(regData)
}
func (r *RegServer) Add(reg *redis_registry.RegServer) {
	r.mx.Lock()
	defer r.mx.Unlock()

	for i := range r.regData {
		if r.regData[i].SeqNo == reg.SeqNo {
			return
		}
	}

	r.regData = append(r.regData, reg)
	r.update(r.regData)
}

func (r *RegServer) update(regData []*redis_registry.RegServer) {
	addrList := makeAddress(regData)
	r.regData = regData
	r.r.UpdateState(resolver.State{Addresses: addrList})
	r.upTime = time.Now().Unix()
}
func (r *RegServer) TryUpdate(regData []*redis_registry.RegServer) {
	r.mx.Lock()
	defer r.mx.Unlock()

	if len(regData) != len(r.regData) {
		r.update(regData)
		return
	}

	// 对比
	for i := range regData {
		if regData[i].SeqNo != regData[i].SeqNo {
			r.update(regData)
			return
		}
	}

	// 仅更新时间
	r.upTime = time.Now().Unix()
}

func (s *RedisDiscover) GetBuilder(ctx context.Context, serverName string) (resolver.Builder, error) {
	s.mx.Lock()
	defer s.mx.Unlock()

	reg, ok := s.res[serverName]
	if ok {
		if len(reg.regData) == 0 {
			return nil, fmt.Errorf("server %s not found router", serverName)
		}
		return reg.r, nil
	}

	regData, err := s.discoverOne(ctx, serverName)
	if err != nil {
		return nil, err
	}

	address := makeAddress(regData)
	r := discover.NewBuilderWithScheme(Type)
	r.InitialState(resolver.State{Addresses: address})
	reg = &RegServer{
		r:       r,
		regData: regData,
		upTime:  time.Now().Unix(),
	}

	signalKey := redis_registry.GenRegSignalKey(serverName)
	err = s.sub.Subscribe(ctx, signalKey)
	if err != nil {
		logger.Log.Error(ctx, "Discover grpc server subscribe err",
			zap.String("RegistryType", Type),
			zap.String("serverName", serverName),
			zap.Error(err),
		)
		return nil, err
	}
	s.res[serverName] = reg
	if len(reg.regData) == 0 {
		return nil, fmt.Errorf("server %s not found router", serverName)
	}
	return reg.r, nil
}

func (s *RedisDiscover) Close() {
	_ = s.sub.Close()
	if s.t != nil {
		s.t.Stop()
	}
}

func (s *RedisDiscover) start() {
	go func() {
		for msg := range s.sub.Channel() {
			if !strings.HasPrefix(msg.Channel, redis_registry.KeyRegSignal) {
				continue
			}
			serverName := msg.Channel[len(redis_registry.KeyRegSignal):]
			s.mx.Lock()
			reg, ok := s.res[serverName]
			s.mx.Unlock()
			if !ok {
				continue
			}

			signal := redis_registry.RegSignal{}
			err := sonic.UnmarshalString(msg.Payload, &signal)
			if err != nil || signal.Reg == nil {
				continue // 抛弃异常信号
			}

			if signal.IsUnReg {
				reg.Remove(signal.Reg.SeqNo)
				continue
			}

			reg.Add(signal.Reg)
		}
	}()

	// 定时主动发现
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
			zap.String("RegistryType", Type),
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
				zap.String("RegistryType", Type),
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
				zap.String("RegistryType", Type),
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
				zap.String("RegistryType", Type),
				zap.String("serverName", serverName),
				zap.Strings("delSeq", delSeq),
				zap.Error(err),
			)
		}
	}
	return ret, nil
}

func makeAddress(regData []*redis_registry.RegServer) []resolver.Address {
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

func (s *RedisDiscover) reDiscoverOne(ctx context.Context, serverName string, reg *RegServer) error {
	nowUnix := time.Now().Unix()
	if nowUnix-reg.upTime < ReDiscoverInterval/2 { // 最近有更新则不需要主动发现
		return nil
	}

	regData, err := s.discoverOne(ctx, serverName)
	if err != nil {
		return err
	}

	reg.TryUpdate(regData)
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
		err := s.reDiscoverOne(ctx, serverName, reg)
		if err != nil {
			logger.Log.Error(ctx, "ReDiscover grpc server err",
				zap.String("DiscoverType", Type),
				zap.String("serverName", serverName),
				zap.Any("reg", reg),
				zap.Error(err),
			)
			return
		}
	}
}

func NewDiscover(app core.IApp, address string) (discover.Discover, error) {
	client := redis.GetClient(address)
	rr := &RedisDiscover{
		client:    client,
		sub:       client.Subscribe(app.BaseContext()),

		res: make(map[string]*RegServer),
	}
	go rr.start()
	return rr, nil
}
