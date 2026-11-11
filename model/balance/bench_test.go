package balance

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc/balancer"
)

type benchCase struct {
	name string
	new  func() strategy
	key  string
}

func benchCases() []benchCase {
	return []benchCase{
		{"round_robin", func() strategy { return new(roundStrategy) }, ""},
		{"weight_random", func() strategy { return new(weightRandomStrategy) }, ""},
		{"weight_hash", func() strategy { return new(weightHashStrategy) }, "user-42"},
		{"ring", func() strategy { return new(ringStrategy) }, "user-42"},
		{"least_request", func() strategy { return new(leastRequestStrategy) }, ""},
	}
}

var benchSizes = []int{8, 100}

// BenchmarkPick 测热路径：每请求一次选点。
//
// 结果写进局部变量后用 KeepAlive 兜住，既避免写全局 sink 的数据竞争，
// 又避免加上原子写的开销污染数字。
func BenchmarkPick(b *testing.B) {
	for _, n := range benchSizes {
		insts := mkInsts(n, 100)
		for _, c := range benchCases() {
			b.Run(fmt.Sprintf("%s/n=%d/seq", c.name, n), func(b *testing.B) {
				s := c.new()
				s.Update(insts)
				b.ReportAllocs()
				b.ResetTimer()

				var out *instance
				for i := 0; i < b.N; i++ {
					inst, err := s.Pick(c.key)
					if err != nil {
						b.Fatal(err)
					}
					out = inst
				}
				runtime.KeepAlive(out)
			})

			b.Run(fmt.Sprintf("%s/n=%d/par", c.name, n), func(b *testing.B) {
				s := c.new()
				s.Update(insts)
				b.ReportAllocs()
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					var out *instance
					for pb.Next() {
						inst, err := s.Pick(c.key)
						if err != nil {
							b.Fatal(err)
						}
						out = inst
					}
					runtime.KeepAlive(out)
				})
			})
		}
	}
}

// BenchmarkUpdate 测地址变化时的重建开销（滚动发布 / 实例上下线走这条）。
func BenchmarkUpdate(b *testing.B) {
	for _, n := range benchSizes {
		insts := mkInsts(n, 100)
		for _, c := range benchCases() {
			b.Run(fmt.Sprintf("%s/n=%d", c.name, n), func(b *testing.B) {
				s := c.new()
				s.Update(insts)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					s.Update(insts)
				}
			})
		}
	}
}

// 并发 Pick 与并发 Update 同时进行：-race 下不能有竞态，不能 panic，
// 不能读到半成品（nil 实例 / nil 子连接）。
func TestConcurrentPickAndUpdate(t *testing.T) {
	setsOf := func() [][]*instance {
		return [][]*instance{mkInsts(8, 100), mkInsts(12, 100), mkInsts(4, 100)}
	}

	for _, c := range benchCases() {
		t.Run(c.name, func(t *testing.T) {
			s := c.new()
			sets := setsOf()
			s.Update(sets[0])

			var (
				wg  sync.WaitGroup
				bad atomic.Int64
			)

			// 写者：固定次数，避免用 stop channel 造成等待环
			const updates = 4000
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < updates; i++ {
					s.Update(sets[i%len(sets)])
				}
			}()

			// 读者
			for g := 0; g < 8; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < 20000; i++ {
						inst, err := s.Pick(c.key)
						if err != nil {
							bad.Add(1)
							continue
						}
						if inst == nil || inst.sc == nil || inst.name == "" {
							bad.Add(1)
						}
					}
				}()
			}
			wg.Wait()

			if n := bad.Load(); n != 0 {
				t.Fatalf("%d 次并发选点拿到无效结果", n)
			}
		})
	}
}

// 在途计数必须守恒：Pick 一次 + Done 一次，最终归零。
// 计数错乱会让 P2C 永久偏向某个实例。
func TestInflightConservation(t *testing.T) {
	insts := mkInsts(4, 100)
	p := readyPicker(t, new(leastRequestStrategy), insts...)

	const (
		goroutines = 16
		perG       = 500
	)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				res, err := p.Pick(balancer.PickInfo{Ctx: context.Background()})
				if err != nil {
					t.Errorf("选点失败: %v", err)
					return
				}
				if res.Done != nil {
					res.Done(balancer.DoneInfo{})
				}
			}
		}()
	}
	wg.Wait()

	for _, inst := range insts {
		if got := inst.inflight.Load(); got != 0 {
			t.Fatalf("%s 在途数残留 %d", inst.name, got)
		}
	}
}
