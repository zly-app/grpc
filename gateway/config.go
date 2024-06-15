/*
-------------------------------------------------
   Author :       zlyuancn
   date：         2021/1/23
   Description :
-------------------------------------------------
*/

package gateway

const (
	// bind地址
	defBind = ":8080"
	// 关闭前等待时间, 单位秒
	defCloseWait = 3
	// 运行跨域
	defCorsAllowAll = true
)

type RouteConfig struct {
	Path            string // 如 /hello/say
	HashKeyByHeader string // 从header中获取hashKey
}

type ServerConfig struct {
	Bind         string // bind地址
	CloseWait    int    // 关闭前等待处理时间, 单位秒
	CorsAllowAll bool   // 跨域

	Route    []*RouteConfig // 路由配置
	routeMap map[string]*RouteConfig
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		CorsAllowAll: defCorsAllowAll,
	}
}

func (conf *ServerConfig) Check() error {
	if conf.Bind == "" {
		conf.Bind = defBind
	}
	if conf.CloseWait < 1 {
		conf.CloseWait = defCloseWait
	}

	conf.routeMap = make(map[string]*RouteConfig, len(conf.Route))
	for _, b := range conf.Route {
		conf.routeMap[b.Path] = b
	}
	return nil
}

func (conf *ServerConfig) GetRouteConfig(path string) (*RouteConfig, bool) {
	b, ok := conf.routeMap[path]
	return b, ok
}
