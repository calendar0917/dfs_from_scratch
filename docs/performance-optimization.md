# 性能优化实战：从 2ms 到 2μs

## 一、问题发现

### 1.1 压测结果
```bash
go test -bench=BenchmarkMaster_AssignVolume -benchmem

# 优化前
BenchmarkMaster_AssignVolume-16    601    2000244 ns/op    1114068 B/op    10829 allocs/op
```

**问题分析：**
- 延迟：2ms/op（太慢！）
- 内存：每次分配 1.1MB（太多！）
- 堆分配：每次 10,829 次（灾难！）

### 1.2 根因定位

**代码问题：**
```go
// 原代码：每次 AssignVolume 都触发异步持久化
func (s *MasterServer) AssignVolume(...) {
    s.fileMetadata[req.Filename] = pickedAddresses
    
    // 每次都要启动 goroutine！
    go func() {
        s.SaveToDisk()  // 序列化 + 文件写入
    }()
}
```

**问题：**
1. 每次操作都启动 goroutine（创建成本 ~2μs）
2. 每次都要 JSON 序列化（分配大量内存）
3. 每次都要写文件（系统调用）
4. 使用 map + 全局锁（锁竞争）

---

## 二、优化方案

### 2.1 优化策略

| 优化点 | 原方案 | 新方案 | 效果 |
|--------|--------|--------|------|
| 数据结构 | map + sync.Mutex | sync.Map | 无锁并发 |
| 持久化时机 | 每次操作 | 定期批量 | 减少 IO |
| 持久化触发 | 同步启动 goroutine | 标记+后台 | 减少 goroutine |
| 内存路径 | 直接写文件 | 先写内存，后台刷盘 | 快速返回 |

### 2.2 核心优化：sync.Map

**原代码（map + 锁）：**
```go
type MasterServer struct {
    mu           sync.RWMutex
    nodes        map[string]*NodeInfo
    fileMetadata map[string][]string
}

func (s *MasterServer) AssignVolume(...) {
    s.mu.Lock()                    // 加锁
    s.fileMetadata[filename] = ... // 写操作
    s.mu.Unlock()                  // 解锁
}
```

**优化后（sync.Map）：**
```go
type MasterServer struct {
    nodes        sync.Map  // map[string]*NodeInfo
    fileMetadata sync.Map  // map[string][]string
}

func (s *MasterServer) AssignVolume(...) {
    s.fileMetadata.Store(filename, ...)  // 无锁操作
}
```

**优势：**
- 读多写少场景性能更好
- 无锁并发，减少竞争
- 内置线程安全

### 2.3 核心优化：后台批量持久化

**原代码（每次操作都持久化）：**
```go
func (s *MasterServer) AssignVolume(...) {
    s.fileMetadata[filename] = addresses
    
    go func() {           // 每次创建 goroutine
        s.SaveToDisk()    // 每次序列化+写文件
    }()
}
```

**优化后（标记+后台定时）：**
```go
type MasterServer struct {
    persistMu     sync.Mutex
    persistDirty  bool          // 是否有变更
    persistStopCh chan struct{}
}

// 只标记，不立即持久化
func (s *MasterServer) markDirty() {
    s.persistMu.Lock()
    s.persistDirty = true
    s.persistMu.Unlock()
}

// 后台协程定期持久化
func (s *MasterServer) startBackgroundPersister() {
    ticker := time.NewTicker(5 * time.Second)
    for {
        select {
        case <-ticker.C:
            if s.persistDirty {
                s.SaveToDisk()
                s.persistDirty = false
            }
        }
    }
}

func (s *MasterServer) AssignVolume(...) {
    s.fileMetadata.Store(filename, addresses)
    s.markDirty()  // 只是标记，立即返回
}
```

**优势：**
- 操作立即返回（~2μs）
- 批量持久化（减少 IO 次数）
- 减少 goroutine 创建

---

## 三、优化效果对比

### 3.1 性能数据

| 指标 | 优化前 | 优化后 | 提升倍数 |
|------|--------|--------|----------|
| 延迟 | 2,000,244 ns/op | **2,089 ns/op** | **958x** |
| 内存分配 | 1,114,068 B/op | **388 B/op** | **2,870x** |
| 堆分配次数 | 10,829 allocs/op | **15 allocs/op** | **722x** |
| QPS | ~500 | ~480,000 | **960x** |

### 3.2 延迟分布

```
优化前：
[====2ms====]

优化后：
[=2μs=]
```

### 3.3 火焰图对比（概念）

**优化前：**
```
100% AssignVolume
  ├── 50% JSON Marshal
  ├── 30% File Write
  ├── 15% Goroutine Create
  └── 5% Business Logic
```

**优化后：**
```
100% AssignVolume
  ├── 95% Business Logic
  └── 5% sync.Map Store
```

---

## 四、关键代码解析

### 4.1 sync.Map 使用模式

```go
// 写操作
s.nodes.Store(key, value)
s.fileMetadata.Store(key, value)

// 读操作
value, ok := s.nodes.Load(key)
if ok {
    node := value.(*NodeInfo)
}

// 遍历（Range 回调中不能修改 map）
s.nodes.Range(func(key, value interface{}) bool {
    id := key.(string)
    info := value.(*NodeInfo)
    // 处理...
    return true  // 继续遍历
})

// 删除
s.nodes.Delete(key)
```

**注意事项：**
- 需要类型断言
- Range 回调中不能修改 map
- 不适合写多读少场景

### 4.2 后台持久化模式

```go
// 1. 定义结构
type Server struct {
    persistMu     sync.Mutex
    persistDirty  bool
    persistStopCh chan struct{}
}

// 2. 启动后台协程
func NewServer() *Server {
    s := &Server{persistStopCh: make(chan struct{})}
    go s.startBackgroundPersister()
    return s
}

// 3. 后台循环
func (s *Server) startBackgroundPersister() {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            s.persistIfDirty()
        case <-s.persistStopCh:
            return
        }
    }
}

// 4. 条件持久化
func (s *Server) persistIfDirty() {
    s.persistMu.Lock()
    if !s.persistDirty {
        s.persistMu.Unlock()
        return
    }
    s.persistDirty = false
    s.persistMu.Unlock()
    
    s.SaveToDisk()
}

// 5. 标记变更
func (s *Server) modifyData() {
    // 修改数据...
    s.markDirty()
}

func (s *Server) markDirty() {
    s.persistMu.Lock()
    s.persistDirty = true
    s.persistMu.Unlock()
}
```

### 4.3 优雅关闭

```go
func (s *MasterServer) Close() {
    close(s.persistStopCh)  // 通知后台协程退出
    s.SaveToDisk()          // 最后保存一次
}
```

---

## 五、优化技巧总结

### 5.1 延迟优化 checklist

- [ ] 避免在热路径创建 goroutine
- [ ] 避免在热路径做序列化
- [ ] 避免在热路径做文件 IO
- [ ] 使用无锁数据结构（sync.Map）
- [ ] 批量处理而非单次处理
- [ ] 异步化非关键操作

### 5.2 内存优化 checklist

- [ ] 使用 sync.Pool 复用对象
- [ ] 预分配 slice 容量
- [ ] 避免在热路径分配内存
- [ ] 使用更高效的序列化（protobuf 替代 JSON）

### 5.3 并发优化 checklist

- [ ] 减少锁粒度
- [ ] 使用读写分离（RWMutex）
- [ ] 使用原子操作替代锁
- [ ] 使用 channel 替代共享内存

---

## 六、如何发现性能问题

### 6.1 Benchmark

```go
func BenchmarkXxx(b *testing.B) {
    b.ReportAllocs()  // 报告内存分配
    
    s := NewServer()
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        s.Operation()
    }
}
```

### 6.2 CPU Profile

```bash
go test -bench=. -cpuprofile=cpu.prof
go tool pprof -http=:8080 cpu.prof
```

### 6.3 火焰图分析

**查看火焰图：**
1. 找到最宽的函数（耗时最多）
2. 分析调用链
3. 定位瓶颈

---

## 七、常见误区

### ❌ 误区 1：过度优化

**不要为了优化而优化，先测量再优化。**

```go
// 没必要：为了省一次内存分配，代码变得复杂难懂
var pool = sync.Pool{
    New: func() interface{} { return make([]byte, 1024) },
}

// 简单清晰更重要
buf := make([]byte, 1024)
```

### ❌ 误区 2：过早优化

**先让代码正确，再让代码快。**

### ❌ 误区 3：忽视可读性

**优化后的代码应该仍然可读。**

---

## 八、学习资源

- Go 性能优化官方博客：https://go.dev/blog/profiling-go-programs
- Go 高性能编程：https://dave.cheney.net/high-performance-go-workshop/gopherchina-2019.html
- sync.Map 源码分析：https://medium.com/@deckarep/the-new-kid-in-town-gos-sync-map-de24a6bf7c2c

