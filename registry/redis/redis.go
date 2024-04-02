package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/cast"
	"github.com/zly-app/component/redis"
	"github.com/zly-app/zapp/core"
	"github.com/zly-app/zapp/logger"
	"github.com/zly-app/zapp/pkg/utils"
	"go.uber.org/zap"

	"github.com/zly-app/grpc/pkg"
	"github.com/zly-app/grpc/registry"
)

const Name = "redis"

const (
	RegistryExpire = 30 // 注册有效时间, 单位秒
	ReRegInterval  = 10 // 重新注册间隔时间, 单位秒
)

const (
	KeySeqIncr   = "grpc:server:seq:"    // 服务申请序号自增号
	KeyServerReg = "grpc:server:reg:"    // 服务注册地址
	KeyRegSignal = "grpc:server:signal:" // 注册型号通道
)

func init() {
	registry.AddCreator(Name, NewRegistry)
}

type RedisRegistry struct {
	creator redis.IRedisCreator
	client  redis.UniversalClient
	t       *time.Ticker

	servers map[string]*RegServer
	mx      sync.Mutex
}

type RegServer struct {
	SeqNo    int    // 序列号
	Name     string // 名称, 用于直接指定目标
	Endpoint string
	Weight   uint16 // 权重
	Deadline int64  // 截止时间
}

// 注册信号
type RegSignal struct {
	Reg     *RegServer
	IsUnReg bool // 是否为取消注册
}

func GenSeqKey(serverName string) string {
	return KeySeqIncr + serverName
}

func GenRegKey(serverName string) string {
	return KeyServerReg + serverName
}

func GenRegField(seq int) string {
	return cast.ToString(seq)
}

func GenRegSignalKey(serverName string) string {
	return KeyRegSignal + serverName
}

func (s *RedisRegistry) Registry(ctx context.Context, serverName string, addr *pkg.AddrInfo) error {
	seqKey := GenSeqKey(serverName)
	seqNo, err := s.client.Incr(ctx, seqKey).Result()
	if err != nil {
		logger.Log.Error(ctx, "Registry grpc server. apply SeqNo err",
			zap.String("RegistryType", Name),
			zap.String("serverName", serverName),
			zap.Any("addr", addr),
			zap.Error(err),
		)
		return err
	}
	reg := &RegServer{
		SeqNo:    int(seqNo),
		Name:     fmt.Sprintf("%s.%d", serverName, seqNo),
		Endpoint: addr.Endpoint,
		Weight:   addr.Weight,
	}

	err = s.registryOne(ctx, serverName, reg)
	if err != nil {
		return err
	}

	signalKey := GenRegSignalKey(serverName)
	signalText, _ := sonic.MarshalString(&RegSignal{
		Reg:     reg,
		IsUnReg: false,
	})
	err = s.client.Publish(ctx, signalKey, signalText).Err()
	if err != nil {
		logger.Log.Error(ctx, "Registry grpc server. publish reg signal err",
			zap.String("RegistryType", Name),
			zap.String("serverName", serverName),
			zap.Any("addr", reg),
			zap.Error(err),
		)
		// 忽略异常
	}

	s.mx.Lock()
	defer s.mx.Unlock()

	s.servers[serverName] = reg
	return nil
}
func (s *RedisRegistry) UnRegistry(ctx context.Context, serverName string) {
	s.mx.Lock()
	defer s.mx.Unlock()

	reg, ok := s.servers[serverName]
	if !ok {
		return
	}

	signalKey := GenRegSignalKey(serverName)
	signalText, _ := sonic.MarshalString(&RegSignal{
		Reg:     reg,
		IsUnReg: true,
	})
	err := s.client.Publish(ctx, signalKey, signalText).Err()
	if err != nil {
		logger.Log.Error(ctx, "Registry grpc server. publish reg signal err",
			zap.String("RegistryType", Name),
			zap.String("serverName", serverName),
			zap.Any("addr", reg),
			zap.Error(err),
		)
		// 忽略异常
	}

	delete(s.servers, serverName)
	key, field := GenRegKey(serverName), GenRegField(reg.SeqNo)
	err = s.client.HDel(ctx, key, field).Err()
	if err != nil {
		logger.Log.Error(ctx, "UnRegistry grpc server err",
			zap.String("RegistryType", Name),
			zap.String("serverName", serverName),
			zap.Any("reg", reg),
			zap.Error(err),
		)
		return
	}
}

func (s *RedisRegistry) Close() {
	s.creator.Close()
	if s.t != nil {
		s.t.Stop()
	}
}

func (s *RedisRegistry) start() {
	s.t = time.NewTicker(time.Second * ReRegInterval)
	for range s.t.C {
		s.registryAll()
	}
}

func (s *RedisRegistry) registryOne(ctx context.Context, serverName string, reg *RegServer) error {
	reg.Deadline = time.Now().Unix() + RegistryExpire // 更新时间

	key, field := GenRegKey(serverName), GenRegField(reg.SeqNo)
	data, _ := sonic.MarshalString(reg)
	err := s.client.HSet(ctx, key, field, data).Err()
	if err != nil {
		logger.Log.Error(ctx, "Registry grpc server err",
			zap.String("RegistryType", Name),
			zap.String("serverName", serverName),
			zap.Any("reg", reg),
			zap.Error(err),
		)
		return err
	}
	return nil
}

func (s *RedisRegistry) registryAll() {
	ctx, span := utils.Otel.StartSpan(context.Background(), "ReRegistryGrpcServer")
	defer utils.Otel.EndSpan(span)

	s.mx.Lock()
	defer s.mx.Unlock()

	for serverName, reg := range s.servers {
		err := s.registryOne(ctx, serverName, reg)
		if err != nil {
			logger.Log.Error(ctx, "ReRegistry grpc server err",
				zap.String("RegistryType", Name),
				zap.String("serverName", serverName),
				zap.Any("reg", reg),
				zap.Error(err),
			)
			return
		}
	}
}

func NewRegistry(app core.IApp, address string) (registry.Registry, error) {
	creator := redis.NewRedisCreator(app)
	client := creator.GetRedis(address)
	rr := &RedisRegistry{
		creator: creator,
		client:  client,
		servers: make(map[string]*RegServer),
	}
	go rr.start()
	return rr, nil
}
