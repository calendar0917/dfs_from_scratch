# Go 性能测试（Benchmark）完全指南

## 一、为什么要做性能测试？

### 1.1 场景
- 优化前：需要**量化**当前性能
- 优化后：验证优化**是否有效**
- 回归测试：防止性能**退化**
- 容量规划：知道系统能撑多少 QPS

### 1.2 性能指标

| 指标 | 说明 | 关注重点 |
|------|------|----------|
| **ns/op** | 每次操作耗时 | 越小越好 |
| **B/op** | 每次操作内存分配 | 越小越好 |
| **allocs/op** | 每次操作堆分配次数 | 越小越好 |
| **MB/s** | 吞吐量（文件操作）| 越大越好 |

---

## 二、Go Benchmark 基础

### 2.1 基本结构

```go
// 文件: xxx_test.go
package xxx

import "testing"

// 函数名必须以 Benchmark 开头
// 参数 *testing.B 表示 benchmark
func BenchmarkXxx(b *testing.B) {
    // 准备数据（不计入 benchmark 时间）
    data := prepareData()
    
    // 重置计时器，正式开始 benchmark
    b.ResetTimer()
    
    // 循环 b.N 次
    for i := 0; i < b.N; i++ {
        // 被测代码
        doSomething(data)
    }
}
```

### 2.2 运行 Benchmark

```bash
# 运行所有 benchmark
go test -bench=.

# 运行特定 benchmark
go test -bench=BenchmarkHash

# 显示内存分配信息
go test -bench=. -benchmem

# 只运行 benchmark，不运行普通测试
go test -bench=. -run=^$

# 指定 benchmark 时间（默认 1s）
go test -bench=. -benchtime=5s

# 指定 CPU 核心数
go test -bench=. -cpu=1,4,8
```

---

## 三、Benchmark 模式详解

### 3.1 串行 Benchmark

```go
func BenchmarkSerial(b *testing.B) {
    s := NewService()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        s.Process()  // 顺序执行
    }
}
```

**结果：**
```
BenchmarkSerial-16    1000000    1000 ns/op
```

### 3.2 并行 Benchmark（多 goroutine）

```go
func BenchmarkParallel(b *testing.B) {
    s := NewService()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        // 每个 goroutine 循环直到 pb.Next() 返回 false
        for pb.Next() {
            s.Process()
        }
    })
}
```

**特点：**
- 自动使用 GOMAXPROCS 个 goroutine
- 模拟真实并发场景
- 测试锁竞争、并发安全

**结果：**
```
BenchmarkParallel-16    5000000    200 ns/op
```

### 3.3 内存分配 Benchmark

```go
func BenchmarkMemory(b *testing.B) {
    b.ReportAllocs()  // 报告内存分配
    
    for i := 0; i < b.N; i++ {
        data := make([]byte, 1024)  // 每次分配 1KB
        process(data)
    }
}
```

**结果：**
```
BenchmarkMemory-16    1000000    100 ns/op    1024 B/op    1 allocs/op
```

### 3.4 设置吞吐量

```go
func BenchmarkThroughput(b *testing.B) {
    data := make([]byte, 1024*1024)  // 1MB
    
    b.SetBytes(int64(len(data)))  // 设置每次处理的数据量
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        process(data)
    }
}
```

**结果：**
```
BenchmarkThroughput-16    1000    1000000 ns/op    1000 MB/s
```

---

## 四、本项目 Benchmark 实战

### 4.1 一致性哈希压测

```go
func BenchmarkConsistentHash_Get(b *testing.B) {
    ch := hash.NewConsistentHash(150)
    ch.Add("node1", "node2", "node3", "node4", "node5")
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        i := 0
        for pb.Next() {
            ch.Get(fmt.Sprintf("file%d.txt", i))
            i++
        }
    })
}
```

**结果分析：**
```
BenchmarkConsistentHash_Get-16    23467051    50.19 ns/op    40 B/op    3 allocs/op
```

| 指标 | 值 | 分析 |
|------|-----|------|
| 23467051 | 1秒内执行了 2300万+ 次 | 非常高效 |
| 50.19 ns/op | 每次 50 纳秒 | 极快（1秒=10亿纳秒）|
| 40 B/op | 每次分配 40 字节 | 较少 |
| 3 allocs/op | 每次 3 次堆分配 | 可优化 |

**优化建议：**
```go
// 优化：复用 buffer，减少分配
var bufPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 64)
    },
}
```

### 4.2 Master 调度压测

```go
func BenchmarkMaster_AssignVolume(b *testing.B) {
    s := service.NewMasterServer(":memory:")
    
    // 注册 5 个节点
    for i := 0; i < 5; i++ {
        s.RegisterNode(ctx, &api.RegisterRequest{...})
    }
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            s.AssignVolume(ctx, &api.AssignVolumeRequest{...})
        }
    })
}
```

**结果分析：**
```
BenchmarkMaster_AssignVolume-16    600    1935411 ns/op    1169832 B/op    10820 allocs/op
```

| 指标 | 值 | 分析 |
|------|-----|------|
| 600 | 1秒只执行了 600 次 | **太慢了！** |
| 1935411 ns/op | 每次 1.9 毫秒 | 应该是微秒级 |
| 1169832 B/op | 每次分配 1.1 MB | **太多了！** |
| 10820 allocs/op | 每次 1万+ 次分配 | **灾难！** |

**问题诊断：**
1. 使用了 `:memory:` 数据库，但每次操作都尝试持久化
2. 异步持久化失败，重试消耗资源
3. 锁竞争严重

**优化方向：**
```go
// 1. 禁用持久化（测试时）
if dbPath == ":memory:" {
    s.persistEnabled = false
}

// 2. 批量持久化
// 3. 使用 sync.Map 替代 map+锁
```

### 4.3 文件读写压测

```go
func BenchmarkVolume_WriteFile(b *testing.B) {
    tmpDir := b.TempDir()
    data := make([]byte, 64*1024)  // 64KB
    
    b.SetBytes(int64(len(data)))
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        path := filepath.Join(tmpDir, fmt.Sprintf("file%d", i))
        os.WriteFile(path, data, 0644)
    }
}
```

**结果分析：**
```
BenchmarkVolume_WriteFile-16    90111    14098 ns/op    4648.70 MB/s    256 B/op    5 allocs/op
```

| 指标 | 值 | 分析 |
|------|-----|------|
| 90111 | 1秒 9万+ 次 | 不错 |
| 14098 ns/op | 每次 14 微秒 | 合理 |
| 4648.70 MB/s | 吞吐量 4.6 GB/s | 很高（SSD）|
| 5 allocs/op | 每次 5 次分配 | 可接受 |

---

## 五、压测结果对比分析

### 5.1 本项目各组件性能

| 组件 | 操作 | 延迟 | 内存分配 | 评价 |
|------|------|------|----------|------|
| 一致性哈希 | Get | 50 ns | 40 B | ⭐⭐⭐⭐⭐ 优秀 |
| 一致性哈希 | GetN | 90 ns | 88 B | ⭐⭐⭐⭐⭐ 优秀 |
| Master | AssignVolume | 1.9 ms | 1.1 MB | ⭐⭐ 需优化 |
| Master | RegisterNode | - | - | 待测试 |
| Volume | WriteFile | 14 μs | 256 B | ⭐⭐⭐⭐ 良好 |
| Volume | ReadFile | 131 μs | 1 MB | ⭐⭐⭐ 一般 |

### 5.2 性能目标参考

| 系统类型 | QPS | 延迟(P99) | 说明 |
|----------|-----|-----------|------|
| 高性能缓存 | 100K+ | < 1ms | Redis 级别 |
| 分布式存储 | 10K+ | < 10ms | 本项目目标 |
| 普通 Web | 1K+ | < 100ms | 一般服务 |

**当前估算：**
- Master AssignVolume: ~500 QPS (1/1.9ms)
- 需要优化到 10K+ QPS

---

## 六、高级技巧

### 6.1 对比测试（A/B 测试）

```go
func BenchmarkOld(b *testing.B) {
    // 旧实现
    for i := 0; i < b.N; i++ {
        oldImplementation()
    }
}

func BenchmarkNew(b *testing.B) {
    // 新实现
    for i := 0; i < b.N; i++ {
        newImplementation()
    }
}
```

**运行对比：**
```bash
go test -bench="BenchmarkOld|BenchmarkNew" -benchmem
```

### 6.2 Profiling 结合

```bash
# CPU Profile
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Memory Profile
go test -bench=. -memprofile=mem.prof
go tool pprof mem.prof

# 查看火焰图
go tool pprof -http=:8080 cpu.prof
```

### 6.3 压力测试（持续运行）

```go
func BenchmarkStress(b *testing.B) {
    // 持续运行 5 分钟
    deadline := time.Now().Add(5 * time.Minute)
    
    for time.Now().Before(deadline) {
        doWork()
    }
}
```

---

## 七、常见问题

### Q1: b.N 是多少？

Go 会自动调整 b.N，使 benchmark 运行时间达到 **1秒**（默认）。

```
第一次：N=1，测时间
第二次：N=100，测时间
... 
直到运行时间接近 1秒
```

### Q2: 为什么结果不稳定？

**原因：**
- CPU 降频/睿频
- 其他进程干扰
- GC 影响

**解决：**
```bash
# 运行多次取平均
go test -bench=. -count=5

# 延长运行时间
go test -bench=. -benchtime=5s
```

### Q3: 如何测试并发性能？

```go
func BenchmarkConcurrent(b *testing.B) {
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            // 每个 goroutine 执行
        }
    })
}
```

`-16` 表示 GOMAXPROCS=16（使用 16 个线程）。

### Q4: Benchmark 和单元测试的区别？

| 特性 | 单元测试 | Benchmark |
|------|----------|-----------|
| 函数名 | TestXxx | BenchmarkXxx |
| 参数 | *testing.T | *testing.B |
| 目的 | 验证正确性 | 测量性能 |
| 运行 | go test | go test -bench=. |
| 结果 | PASS/FAIL | ns/op, B/op |

---

## 八、最佳实践

1. **准备数据不计时**
   ```go
   data := prepare()  // 在 ResetTimer 之前
   b.ResetTimer()
   ```

2. **避免优化掉被测代码**
   ```go
   result := process()
   _ = result  // 使用结果，防止被优化掉
   ```

3. **测试不同输入规模**
   ```go
   func BenchmarkProcess1K(b *testing.B)  { benchmarkProcess(1024, b) }
   func BenchmarkProcess1M(b *testing.B)  { benchmarkProcess(1024*1024, b) }
   ```

4. **记录关键指标**
   ```go
   b.Logf("Cache hit rate: %f", hitRate)
   ```


---

## 九、本项目压测结果汇总

### 9.1 运行命令

```bash
# 运行所有 benchmark
cd /home/calendar/code/go-dfs
go test ./internal/bench/... -bench=. -benchmem -run=^$

# 运行特定组件
go test ./internal/bench/... -bench=BenchmarkConsistent -benchmem
go test ./internal/bench/... -bench=BenchmarkMaster -benchmem
go test ./internal/bench/... -bench=BenchmarkVolume -benchmem
```

### 9.2 关键发现

**✅ 优秀性能：**
- 一致性哈希 Get: 50 ns/op（极快）
- 一致性哈希 GetN: 90 ns/op（极快）

**⚠️ 需要优化：**
- Master AssignVolume: 1.9 ms/op（太慢，目标 < 100 μs）
- 原因：持久化逻辑阻塞、锁竞争

**✅ 良好性能：**
- Volume 写文件: 14 μs/op, 4.6 GB/s
- Volume 读文件: 131 μs/op, 8 GB/s

### 9.3 优化建议优先级

1. **高优先级**：优化 Master AssignVolume
   - 禁用测试时的持久化
   - 使用 sync.Map 替代 map+锁
   - 批量异步持久化

2. **中优先级**：优化 Volume 读性能
   - 使用更大的 buffer
   - 预读取（readahead）

3. **低优先级**：减少内存分配
   - 使用对象池（sync.Pool）
   - 复用 buffer

