// Package registry 是模型层 —— 注册实现与静态地址表的所有者。
//
// 拥有：进程内静态地址表、Redis 注册器与本进程注册记录的本地副本、注册器目录。
// 提供：StaticTable / RedisRegistry / Directory。
// 允许调用：foundation/addr、foundation/port、foundation/resolver、model/endpoint。
// 禁止：让发现侧依赖本包；横向访问只经 port.StaticTable / port.Registry。
//
// 唯一施放入口：Register / Unregister 内完成「状态生效 + 通知发出」，
// 不允许外部只写状态不发信号。发现侧只读，不做任何删除或过期判定。
package registry
