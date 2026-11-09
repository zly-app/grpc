package app

import (
	"fmt"

	"github.com/zly-app/zapp/core"
)

type role uint8

const (
	roleServer role = iota
	roleGateway
)

// serviceAdapter 把组合根的两个用例委托给 zapp 的 core.IService 生命周期。
// 壳只做委托，不在里面写时序或规则。
type serviceAdapter struct {
	rt   *App
	role role
}

func (s *serviceAdapter) Inject(...interface{}) {}

func (s *serviceAdapter) Start() error {
	if s.rt == nil {
		return fmt.Errorf("grpc 组合根未创建")
	}
	switch s.role {
	case roleServer:
		if s.rt.serve == nil {
			return fmt.Errorf("grpc 服务端未装配：请把 Options() 传给 zapp.NewApp")
		}
		return s.rt.serve.Start()
	case roleGateway:
		if s.rt.gateway == nil {
			return fmt.Errorf("grpc 网关未装配：请把 Options() 传给 zapp.NewApp")
		}
		return s.rt.gateway.StartGateway()
	}
	return nil
}

func (s *serviceAdapter) Close() error {
	// 关闭顺序由组合根的 Close 统一编排（挂在 BeforeExitHandler），此处不重复关闭。
	return nil
}

var _ core.IService = (*serviceAdapter)(nil)
