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
)

type ServerConfig struct {
	Bind      string // bind地址
	CloseWait int    // 关闭前等待时间, 单位秒
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{}
}

func (conf *ServerConfig) Check() error {
	if conf.Bind == "" {
		conf.Bind = defBind
	}
	if conf.CloseWait < 1 {
		conf.CloseWait = defCloseWait
	}
	return nil
}
