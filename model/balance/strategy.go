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
	"time"
	"unsafe"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	ewma     atomic.Int64 // 平滑后的 RPC 延迟（纳秒），0 表示还没有样本
	sampled  atomic.Int64 // 最后一次采样时刻（UnixNano），用于让样本过期
	fails    atomic.Int32 // 连续失败次数，成功一次清零
}

// done 由 gRPC 在 RPC 结束时调用（成功失败都会调），用于回收 in-flight 计数。
func (i *instance) done(balancer.DoneInfo) { i.inflight.Add(-1) }

// EWMA 的衰减速度：每来一个样本，向新值靠拢 1/ewmaDen。
const ewmaDen = 5

// 内部参数的默认值，可用 SetTuning / SetRingReplicas 覆盖。
const (
	defaultSampleTTL     = 30 * time.Second
	defaultFailThreshold = 3
	defaultRingReplicas  = 160
)

// sampleTTL 是延迟样本的保鲜期。
//
// 节点一旦被持续降权就再也拿不到新样本，旧的高延迟会永久保留、把它饿死。
// 过期后回到「未知」，靠权重随机重新采样。
var sampleTTL atomic.Int64

// failThreshold 是连续失败几次开始降权。一次失败可能只是抖动，所以要给个阈值。
var failThreshold atomic.Int32

// ringReplicas 是一致性 hash 环上每个实例的虚拟节点数（按权重等比缩放）。
var ringReplicas atomic.Int32

func init() {
	sampleTTL.Store(int64(defaultSampleTTL))
	failThreshold.Store(defaultFailThreshold)
	ringReplicas.Store(defaultRingReplicas)
}

// Tuning 是 P2C 的内部参数，零值字段保持默认。
type Tuning struct {
	// SampleTTL 是延迟样本的保鲜期。节点被持续降权就拿不到新样本，
	// 旧的高延迟会永久保留、把它饿死；过期后回到「未知」重新采样。
	SampleTTL time.Duration
	// FailThreshold 是连续失败几次开始降权。
	FailThreshold int32
}

// SetTuning 调整 P2C 的内部参数；零值字段保持当前值。进程级生效，建议在启动期调用。
func SetTuning(t Tuning) {
	if t.SampleTTL > 0 {
		sampleTTL.Store(int64(t.SampleTTL))
	}
	if t.FailThreshold > 0 {
		failThreshold.Store(t.FailThreshold)
	}
}

// SetRingReplicas 调整一致性 hash 环上每个实例的虚拟节点数（按权重等比缩放）。
//
// 默认 160（ketama 标准），对绝大多数场景足够。实例数很多（数百以上）且要求极均匀时
// 可以调大，代价是内存与环重建耗时线性增长；调小会让分布更容易偏斜。
// n <= 0 恢复默认。只影响此后的环重建，已在运行的 ClientConn 会在下次地址变化时生效。
func SetRingReplicas(n int) {
	if n <= 0 {
		n = defaultRingReplicas
	}
	ringReplicas.Store(int32(n))
}

// observe 记录一次 RPC 的耗时并更新 EWMA。
//
// 失败请求不计入延迟：错误通常在连接/协议层快速返回，会把延迟拉低，
// 反而让故障节点显得更快。但它们要按次数累积，用来降权——否则「快速失败」
// 的节点会被 P2C 当成最快节点而拿到最多流量。
func (i *instance) observe(d time.Duration, err error) {
	if err != nil {
		if isNodeFailure(err) {
			// 节点错误不采样延迟：错误通常快速返回，采进去会把延迟拉低、
			// 让故障节点显得更快。
			i.fails.Add(1)
			return
		}
		// 业务错误说明节点是活的：清零失败计数，且延迟照常采样。
		// 不采样会让它永远「没有样本」，被 loadOf 当成最快节点抢走流量。
		i.fails.Store(0)
	} else {
		i.fails.Store(0)
	}

	if d <= 0 {
		return
	}
	ns := int64(d)
	if old := i.ewma.Load(); old > 0 {
		ns = old + (ns-old)/ewmaDen
	}
	i.ewma.Store(ns)
	i.sampled.Store(time.Now().UnixNano())
}

// isNodeFailure 判断一个错误是否该算作「节点问题」。
//
// 只认服务端自身的问题：连不上、内部错误、超时、被限流、数据损坏，以及无法分类的错误
// （`Unknown`——非 status 错误在 gRPC 层通常来自传输/编解码）。
// 业务错误（参数错、找不到、无权限、客户端主动取消）是调用方的问题，拿来降权会让整个
// 集群一起被降权，P2C 最终退化成随机，反而掩盖真正的故障。
//
// 走 Unwrap 链而不是 status.Code：拦截器/hook 可能用 %w 包装错误，
// status.Code 只认直接实现 GRPCStatus 的那一层，包装后一律返回 Unknown，
// 业务错误就会被误判成节点问题（真机实测过：业务错误实例在第 3 次后被完全排除）。
func isNodeFailure(err error) bool {
	code := codes.Unknown
	for e := err; e != nil; e = errors.Unwrap(e) {
		if se, ok := e.(interface{ GRPCStatus() *status.Status }); ok {
			code = se.GRPCStatus().Code()
			break
		}
	}

	switch code {
	case codes.Unavailable, codes.Internal, codes.DeadlineExceeded,
		codes.ResourceExhausted, codes.DataLoss, codes.Unknown:
		return true
	}
	return false
}

// failedRank 返回降权用的失败级别；未达到阈值返回 0（正常参与负载比较）。
func (i *instance) failedRank() int32 {
	if f := i.fails.Load(); f >= failThreshold.Load() {
		return f
	}
	return 0
}

// latency 返回可用于比较的延迟估计；0 表示「没有新鲜样本」。
// now 由调用方传入（一次 Pick 只取一次时间），避免每个候选各调一次 time.Now。
func (i *instance) latency(now int64) int64 {
	lat := i.ewma.Load()
	if lat <= 0 {
		return 0
	}
	if now-i.sampled.Load() > sampleTTL.Load() {
		return 0
	}
	return lat
}

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
	// 虚拟节点数按权重等比缩放，而不是直接等于权重——否则调权重会连带动环的精度。
	base := float64(ringReplicas.Load())
	replicas := make([]int, len(insts))
	total := 0
	for i, inst := range insts {
		n := int(math.Round(base * float64(weightOf(inst)) / float64(addr.DefWeight)))
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

	now := time.Now().UnixNano() // 两个候选共用一次取时
	best, cand := insts[i], insts[j]

	ra, rb := best.failedRank(), cand.failedRank()
	lb, lc := loadOf(best, now), loadOf(cand, now)

	// 连续失败的节点先出局：快速失败会拉低延迟样本，只比 load 会让它拿到更多流量。
	// 用独立的失败级别比较，而不是把系数乘进 load，避免极端值溢出、语义也清楚。
	if ra != rb {
		if rb < ra {
			return cand, nil
		}
		return best, nil
	}

	if lb != lc {
		if lc < lb {
			return cand, nil
		}
		return best, nil
	}

	// 负载相同（低并发下通常都是 0）时按权重加权随机。
	// 只比 in-flight 的话 PublishWeight 在低 QPS 场景完全失效——
	// 实测权重 200:100 串行请求会退化成 1.03:1。
	return pickWeighted(best, cand), nil
}

// pickWeighted 在 a、b 之间按权重随机。
func pickWeighted(a, b *instance) *instance {
	wa, wb := weightOf(a), weightOf(b)
	if wa+wb == 0 {
		return a
	}
	if rand.Uint32N(wa+wb) < wa {
		return a
	}
	return b
}

// loadOf 是 P2C 的比较依据，越小越优先。
//
// 有延迟样本时用「延迟 × (在途+1) / 权重」：延迟体现节点处理能力，在途数体现瞬时排队，
// 权重让同等条件下的高权重实例更占优。
// 没有样本时退回「在途数 / 权重」；两者都没有（冷启动、串行低 QPS）返回 0，
// 由 Pick 的「负载相同」分支按权重加权随机。
func loadOf(i *instance, now int64) int64 {
	inf := i.inflight.Load()
	lat := i.latency(now)
	if lat <= 0 {
		if inf == 0 {
			return 0
		}
		return inf * 1000 / int64(weightOf(i))
	}
	return lat * (inf + 1) * 1000 / int64(weightOf(i))
}
