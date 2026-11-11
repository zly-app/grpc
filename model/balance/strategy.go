package balance

import (
	"cmp"
	"errors"
	"hash/crc32"
	"math"
	rand "math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"sync/atomic"
	"unsafe"

	"google.golang.org/grpc/balancer"

	"github.com/zly-app/grpc/foundation/addr"
)

// hashString 零拷贝算 crc32。
//
// 直接写 []byte(s) 会让每个请求多一次堆分配（benchmark 里是 8 B/op、1 allocs/op），
// 而一致性 hash 的 key 是每请求都要算的。unsafe.Slice 只读，不持有、不修改原串。
func hashString(s string) uint32 {
	if len(s) == 0 {
		return 0
	}
	return crc32.ChecksumIEEE(unsafe.Slice(unsafe.StringData(s), len(s)))
}

// errNoInstance 表示当前没有可选实例；picker 会把它翻译成 gRPC 的
// ErrNoSubConnAvailable，让 RPC 等待新 picker 而不是直接失败。
var errNoInstance = errors.New("balance: 没有可用实例")

// instance 是一个可选实例。
//
// 对象在 ClientConn 生命周期内复用：in-flight 计数必须跨 picker 重建存活，
// 否则每次地址增减都会把 P2C 的观测清零，退化成随机。
type instance struct {
	id       string // 线缆地址，用作全序兜底（同名实例之间定序）
	sc       balancer.SubConn
	name     string
	weight   uint16
	inflight atomic.Int64
}

// done 由 gRPC 在 RPC 结束时调用（成功失败都会调），用于回收 in-flight 计数。
func (i *instance) done(balancer.DoneInfo) { i.inflight.Add(-1) }

// strategy 是选点算法。Update 只在「可用实例集合或权重」变化时调用，用于预计算
// （权重线段、hash 环）；Pick 只读，必须并发安全。
type strategy interface {
	Update(insts []*instance)
	Pick(hashKey string) (*instance, error)
}

// instList 是所有策略共享的不可变快照：Update 换指针，Pick 无锁读。
type instList struct{ v atomic.Pointer[[]*instance] }

func (l *instList) Update(insts []*instance) { l.v.Store(&insts) }

func (l *instList) load() []*instance {
	p := l.v.Load()
	if p == nil {
		return nil
	}
	return *p
}

// weightOf 兜底权重：0 在加权策略里等于「摘掉该实例」，绝不能让零值参与计算。
func weightOf(i *instance) uint32 {
	if i.weight == 0 {
		return uint32(addr.DefWeight)
	}
	return uint32(i.weight)
}

// ---------- 轮询 ----------

type roundStrategy struct {
	instList
	incr atomic.Uint64
}

func (s *roundStrategy) Update(insts []*instance) { s.instList.Update(insts) }

func (s *roundStrategy) Pick(string) (*instance, error) {
	return pickRound(s.load(), &s.incr)
}

// pickRound 的计数器独立于 picker 存在（挂在策略上），地址变化不会把轮询位置清零。
func pickRound(insts []*instance, incr *atomic.Uint64) (*instance, error) {
	l := len(insts)
	if l == 0 {
		return nil, errNoInstance
	}
	if l == 1 {
		return insts[0], nil
	}
	n := incr.Add(1) - 1
	if l&(l-1) == 0 {
		return insts[int(n&uint64(l-1))], nil
	}
	return insts[int(n%uint64(l))], nil
}

// ---------- 加权线段（加权随机 / 加权 hash 共用） ----------

// segments 把实例按权重首尾相接切成若干线段，用累计权重终点做二分查找。
type segments struct {
	insts []*instance
	ends  []uint32
	total uint32
}

func buildSegments(insts []*instance) *segments {
	sg := &segments{
		insts: make([]*instance, 0, len(insts)),
		ends:  make([]uint32, 0, len(insts)),
	}
	for _, i := range insts {
		sg.total += weightOf(i)
		sg.insts = append(sg.insts, i)
		sg.ends = append(sg.ends, sg.total)
	}
	return sg
}

func (sg *segments) at(score uint32) *instance {
	idx := sort.Search(len(sg.ends), func(i int) bool { return sg.ends[i] > score })
	if idx >= len(sg.insts) {
		idx = len(sg.insts) - 1
	}
	return sg.insts[idx]
}

// ---------- 加权随机 ----------

type weightRandomStrategy struct {
	instList
	sg atomic.Pointer[segments]
}

func (s *weightRandomStrategy) Update(insts []*instance) {
	s.instList.Update(insts)
	s.sg.Store(buildSegments(insts))
}

func (s *weightRandomStrategy) Pick(string) (*instance, error) {
	sg := s.sg.Load()
	if sg == nil || sg.total == 0 {
		return nil, errNoInstance
	}
	if len(sg.insts) == 1 {
		return sg.insts[0], nil
	}
	return sg.at(rand.Uint32N(sg.total)), nil
}

// ---------- 加权 hash ----------
//
// 注意这不是一致性 hash：score 对「总权重」取模，实例一增一减就会改变总分母，
// 几乎所有 key 都会换实例。要稳定映射请用 WeightConsistentHash。

type weightHashStrategy struct {
	instList
	sg   atomic.Pointer[segments]
	incr atomic.Uint64
}

func (s *weightHashStrategy) Update(insts []*instance) {
	s.instList.Update(insts)
	s.sg.Store(buildSegments(insts))
}

func (s *weightHashStrategy) Pick(hashKey string) (*instance, error) {
	sg := s.sg.Load()
	if sg == nil || sg.total == 0 {
		return nil, errNoInstance
	}
	if len(sg.insts) == 1 {
		return sg.insts[0], nil
	}
	if hashKey == "" {
		return pickRound(s.load(), &s.incr) // 无 key 时降级为轮询
	}
	return sg.at(hashString(hashKey) % sg.total), nil
}

// ---------- 一致性 hash（ketama） ----------

// ringReplicas 是「权重 = DefWeight」时的虚拟节点数。参考 ketama 的 160。
// 虚拟节点数按权重等比缩放，而不是直接等于权重——否则调权重会连带动环的精度。
const ringReplicas = 160

type ringStrategy struct {
	instList
	ring atomic.Pointer[hashRing]
	incr atomic.Uint64
}

// ringPoint 把虚拟节点的哈希和归属放在一起，省掉「hash → 实例」的 map。
// 100 实例 × 160 副本 = 16000 个点，map 版本每次重建要 2.2 万次分配。
type ringPoint struct {
	hash uint32
	inst *instance
}

type hashRing struct {
	points []ringPoint
}

func buildRing(insts []*instance) *hashRing {
	replicas := make([]int, len(insts))
	total := 0
	for i, inst := range insts {
		n := int(math.Round(float64(ringReplicas) * float64(weightOf(inst)) / float64(addr.DefWeight)))
		if n < 1 {
			n = 1
		}
		replicas[i] = n
		total += n
	}

	r := &hashRing{points: make([]ringPoint, 0, total)}
	buf := make([]byte, 0, 64)
	for i, inst := range insts {
		buf = buf[:0]
		buf = append(buf, inst.name...)
		buf = append(buf, '#')
		prefix := len(buf)
		for n := 0; n < replicas[i]; n++ {
			buf = buf[:prefix]
			buf = strconv.AppendInt(buf, int64(n), 10)
			r.points = append(r.points, ringPoint{hash: crc32.ChecksumIEEE(buf), inst: inst})
		}
	}
	slices.SortFunc(r.points, func(a, b ringPoint) int { return cmp.Compare(a.hash, b.hash) })
	return r
}

func (s *ringStrategy) Update(insts []*instance) {
	s.instList.Update(insts)
	s.ring.Store(buildRing(insts))
}

func (s *ringStrategy) Pick(hashKey string) (*instance, error) {
	r := s.ring.Load()
	if r == nil || len(r.points) == 0 {
		return nil, errNoInstance
	}
	if hashKey == "" {
		return pickRound(s.load(), &s.incr) // 无 key 时降级为轮询
	}
	h := hashString(hashKey)
	idx := sort.Search(len(r.points), func(i int) bool { return r.points[i].hash >= h })
	if idx == len(r.points) {
		idx = 0
	}
	return r.points[idx].inst, nil
}

// ---------- 最少请求（P2C） ----------
//
// 参考 Envoy / brpc 的 least_request：随机抽两个候选，取负载最低的那个。
// 负载 = 在途请求数 / 权重——带权重是为了让 PublishWeight 仍然有效，
// 纯 in-flight 会把权重完全覆盖掉。
//
// 两个候选必须不同：若允许重复，只有 2 个实例时有一半概率两次抽到同一个，
// 等于退化成随机。

type leastRequestStrategy struct{ instList }

func (s *leastRequestStrategy) Update(insts []*instance) { s.instList.Update(insts) }

func (s *leastRequestStrategy) Pick(string) (*instance, error) {
	insts := s.load()
	l := len(insts)
	if l == 0 {
		return nil, errNoInstance
	}
	if l == 1 {
		return insts[0], nil
	}

	i := int(rand.Uint32N(uint32(l)))
	j := int(rand.Uint32N(uint32(l - 1)))
	if j >= i {
		j++ // 在 [0, l-2] 上取第二个下标，跳过 i，保证两者不同
	}

	best, cand := insts[i], insts[j]
	if loadOf(cand) < loadOf(best) {
		best = cand
	}
	return best, nil
}

func loadOf(i *instance) int64 {
	return i.inflight.Load() * 1000 / int64(weightOf(i))
}
