package discover

import (
	"fmt"
	"sync"
	"testing"

	"github.com/zly-app/grpc/model/endpoint"
)

func rec(ep string, weight uint16) *endpoint.Record {
	return &endpoint.Record{Name: ep, Endpoint: ep, Weight: weight}
}

func endpoints(e *entry) []string {
	out := make([]string, 0, len(e.records()))
	for _, v := range e.records() {
		out = append(out, v.Endpoint)
	}
	return out
}

func TestEntryUpsertDedupAndSort(t *testing.T) {
	e := newEntry()
	e.upsert(rec("b", 100))
	e.upsert(rec("a", 100))
	if got := endpoints(e); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("地址必须按 endpoint 排序且不重复, 实际 %v", got)
	}

	e.upsert(rec("a", 300))
	if got := e.records(); len(got) != 2 || got[0].Weight != 300 {
		t.Fatalf("同 endpoint 应更新为最新值, 实际 %+v", got)
	}
}

func TestEntryDrop(t *testing.T) {
	e := newEntry()
	e.upsert(rec("a", 100))
	e.drop("a")
	if !e.empty() {
		t.Fatalf("下线后地址应清空, 实际 %v", endpoints(e))
	}
	e.drop("a") // 重复下线不 panic
}

// 拉取期间到达的信号比快照新：此时快照只能补不能删。
func TestEntryMergeDiscardsStaleSnapshot(t *testing.T) {
	e := newEntry()

	startVer := e.version() // 发起 HGETALL 之前的版本
	e.upsert(rec("b", 100)) // HGETALL 期间实例 b 注册

	e.merge([]*endpoint.Record{rec("a", 100)}, startVer)

	got := endpoints(e)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("旧快照不得摘掉信号带来的实例, 实际 %v", got)
	}
}

// 期间没有信号：快照是权威，必须能删除 TTL 过期的实例。
func TestEntryMergeReplacesWhenVersionUnchanged(t *testing.T) {
	e := newEntry()
	e.upsert(rec("a", 100))
	e.upsert(rec("b", 100))

	startVer := e.version()
	e.merge([]*endpoint.Record{rec("a", 100)}, startVer)

	got := endpoints(e)
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("权威快照应删除已过期实例, 实际 %v", got)
	}
}

// 读者无锁，必须与写者并发安全（配合 -race 使用）。
func TestEntryConcurrentReadWrite(t *testing.T) {
	e := newEntry()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			ep := fmt.Sprintf("10.0.0.%d:1", i)
			for j := 0; j < 200; j++ {
				e.upsert(rec(ep, uint16(100+j)))
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = e.records()
				_ = e.empty()
				_ = e.version()
				_ = e.lastSeen()
			}
		}()
	}
	wg.Wait()

	if n := len(e.records()); n != 8 {
		t.Fatalf("并发写入后应保留 8 条地址, 实际 %d", n)
	}
}
