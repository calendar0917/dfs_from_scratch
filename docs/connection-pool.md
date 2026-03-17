# gRPC 连接池完全指南

## 一、为什么需要连接池？

### 1.1 问题场景
```go
// 每次转发都新建连接
func uploadToNextNode(addr string, data []byte) {
    conn, _ := grpc.Dial(addr, ...)  // 1. 建立 TCP 连接
    client := NewVolumeClient(conn)
    stream, _ := client.UploadFile(...)
    stream.Send(data)
    conn.Close()  // 2. 立即关闭
}
```

**问题：**
- TCP 三次握手耗时（~RTT）
- gRPC HTTP/2 连接建立耗时
- 高并发时连接数爆炸
- 频繁创建/销毁连接开销大

### 1.2 连接池解决方案
```
┌─────────────────────────────────────────┐
│              连接池 (Pool)               │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐   │
│  │ Conn 1  │ │ Conn 2  │ │ Conn 3  │   │
│  │ (idle)  │ │ (busy)  │ │ (idle)  │   │
│  └─────────┘ └─────────┘ └─────────┘   │
│                                         │
│  可用连接队列: [Conn 1, Conn 3]          │
│  最大连接数: 10                          │
│  当前连接数: 3                           │
└─────────────────────────────────────────┘
```

**优势：**
- 连接复用，避免重复建立
- 控制最大连接数，防止资源耗尽
- 连接保活，减少延迟
- 统一管理，便于监控

---

## 二、连接池核心概念

### 2.1 关键参数

| 参数 | 说明 | 建议值 |
|------|------|--------|
| InitialConn | 初始连接数 | 2-5 |
| MaxConn | 最大连接数 | 10-50 |
| MaxIdle | 最大空闲连接 | 5-10 |
| ConnTimeout | 连接超时 | 5s |
| MaxIdleTime | 最大空闲时间 | 10min |
| MaxLifetime | 连接最大生命周期 | 30min |

### 2.2 连接状态流转

```
┌──────────┐    Get()     ┌──────────┐   Release()   ┌──────────┐
│  创建中   │ ───────────> │  使用中   │ ───────────> │  空闲中   │
└──────────┘              └──────────┘              └──────────┘
                               │                          │
                               │ Close()                  │ cleanup()
                               v                          v
                          ┌──────────┐              ┌──────────┐
                          │  已关闭   │              │  已关闭   │
                          └──────────┘              └──────────┘
```

---

## 三、实现详解

### 3.1 核心结构

```go
// 连接池配置
type Config struct {
    InitialConn int           // 初始连接数
    MaxConn     int           // 最大连接数
    MaxIdle     int           // 最大空闲连接
    ConnTimeout time.Duration // 连接超时
    MaxIdleTime time.Duration // 最大空闲时间
    MaxLifetime time.Duration // 连接最大生命周期
}

// 包装后的连接
type PooledConn struct {
    *grpc.ClientConn  // 底层 gRPC 连接
    pool        *GRPCPool
    createdAt   time.Time  // 创建时间
    lastUsedAt  time.Time  // 最后使用时间
    inUse       int32      // 是否在使用中（原子操作）
}

// 连接池
type GRPCPool struct {
    mu          sync.RWMutex
    address     string              // 目标地址
    dialOptions []grpc.DialOption   // gRPC 拨号选项
    
    conns       []*PooledConn       // 所有连接
    available   chan *PooledConn    // 可用连接队列
    
    config      *Config
    closed      int32               // 是否已关闭
    connCount   int32               // 当前连接数
}
```

### 3.2 获取连接流程

```go
func (p *GRPCPool) Get(ctx context.Context) (*PooledConn, error) {
    // 1. 检查池是否已关闭
    if p.closed {
        return nil, ErrPoolClosed
    }
    
    // 2. 尝试从可用队列获取
    select {
    case conn := <-p.available:
        if conn.IsHealthy() {
            conn.inUse = 1
            return conn, nil
        }
        // 连接不健康，移除并重试
        p.removeConn(conn)
        return p.Get(ctx)
        
    case <-ctx.Done():
        return nil, ctx.Err()
        
    default:
        // 3. 队列为空，尝试创建新连接
        if p.connCount < p.config.MaxConn {
            conn, err := p.createConn()
            if err != nil {
                return nil, err
            }
            p.connCount++
            conn.inUse = 1
            return conn, nil
        }
        
        // 4. 达到最大连接数，阻塞等待
        select {
        case conn := <-p.available:
            conn.inUse = 1
            return conn, nil
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }
}
```

**流程图：**
```
Get()
  │
  ├─> 池已关闭? ──> 返回 ErrPoolClosed
  │
  ├─> 尝试从 available 获取 ──> 健康? ──> 标记 inUse=1 ──> 返回
  │                              │
  │                              └─> 不健康 ──> 移除 ──> 重试
  │
  ├─> 队列为空 ──> 当前连接 < MaxConn? ──> 创建新连接 ──> 返回
  │                              │
  │                              └─> 已达到上限 ──> 阻塞等待
  │
  └─> 超时 ──> 返回 ctx.Err()
```

### 3.3 归还连接流程

```go
func (c *PooledConn) Release() {
    // 1. 标记为未使用
    c.inUse = 0
    c.lastUsedAt = time.Now()
    
    // 2. 放回可用队列
    select {
    case c.pool.available <- c:
        // 成功放入
    default:
        // 队列满了，直接关闭
        c.pool.removeConn(c)
    }
}
```

**关键点：**
- 必须调用 `Release()` 归还，否则连接泄漏
- 使用 `defer conn.Release()` 确保归还
- 队列满时直接关闭，避免内存泄漏

### 3.4 连接清理

```go
// 定期清理过期连接
func (p *GRPCPool) cleanup() {
    ticker := time.NewTicker(30 * time.Second)
    for {
        select {
        case <-ticker.C:
            p.cleanupIdleConns()
        case <-p.stopCh:
            return
        }
    }
}

func (p *GRPCPool) cleanupIdleConns() {
    for _, conn := range p.conns {
        if conn.inUse == 1 {
            continue  // 跳过使用中的连接
        }
        
        // 检查是否需要清理
        shouldRemove := false
        
        // 超过最大生命周期
        if now.Sub(conn.createdAt) > p.config.MaxLifetime {
            shouldRemove = true
        }
        
        // 超过最大空闲时间
        if now.Sub(conn.lastUsedAt) > p.config.MaxIdleTime {
            shouldRemove = true
        }
        
        // 连接不健康
        if !conn.IsHealthy() {
            shouldRemove = true
        }
        
        if shouldRemove {
            p.removeConn(conn)
        }
    }
}
```

---

## 四、使用方式

### 4.1 基本使用

```go
// 1. 创建连接池
pool, err := pool.NewGRPCPool(
    "localhost:50052",
    &pool.Config{
        InitialConn: 2,
        MaxConn:     10,
        ConnTimeout: 5 * time.Second,
    },
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
if err != nil {
    log.Fatal(err)
}

// 2. 获取连接
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

conn, err := pool.Get(ctx)
if err != nil {
    log.Fatal(err)
}

// 3. 使用连接
client := api.NewVolumeServiceClient(conn.ClientConn)
stream, err := client.UploadFile(ctx)
// ... 使用 stream

// 4. 归还连接（必须！）
defer conn.Release()
```

### 4.2 在 VolumeServer 中集成

```go
type VolumeServer struct {
    StorageDir string
    connPool   map[string]*pool.GRPCPool // 每个下游地址一个池
}

func (s *VolumeServer) getPool(address string) (*pool.GRPCPool, error) {
    // 1. 检查是否已有池
    if p, ok := s.connPool[address]; ok {
        return p, nil
    }
    
    // 2. 创建新池
    p, err := pool.NewGRPCPool(address, pool.DefaultConfig())
    if err != nil {
        return nil, err
    }
    
    s.connPool[address] = p
    return p, nil
}

func (s *VolumeServer) UploadFile(stream ...) error {
    // 转发到下一个节点
    if len(metadata.NextTargets) > 1 {
        nextAddr := metadata.NextTargets[1]
        
        // 获取连接池
        p, err := s.getPool(nextAddr)
        if err != nil {
            return err
        }
        
        // 获取连接
        ctx, cancel := context.WithTimeout(...)
        defer cancel()
        
        conn, err := p.Get(ctx)
        if err != nil {
            return err
        }
        defer conn.Release()  // 确保归还
        
        // 使用连接转发
        client := api.NewVolumeServiceClient(conn.ClientConn)
        nextStream, _ := client.UploadFile(ctx)
        // ...
    }
}
```

---

## 五、关键设计决策

### 5.1 为什么用 channel 做可用队列？

```go
available chan *PooledConn
```

**优势：**
- 天然的线程安全（channel 内部有锁）
- 支持阻塞等待（`<-p.available`）
- 支持非阻塞尝试（`select { case conn := <-p.available: ... default: ... }`）

### 5.2 为什么用原子操作标记 inUse？

```go
inUse int32  // 使用 atomic.StoreInt32/LoadInt32
```

**优势：**
- 比互斥锁更轻量
- 适合简单的状态标记
- 避免死锁风险

### 5.3 为什么需要定期清理？

**问题：**
- 连接可能因网络问题变"僵尸"
- 长期空闲连接占用资源
- 服务端可能已关闭连接

**解决方案：**
- 定期检查连接健康状态
- 关闭超过生命周期的连接
- 保持连接池"新鲜"

---

## 六、常见问题

### Q1: 连接泄漏怎么办？

**症状：** 连接数持续增长，不下降

**原因：**
- 忘记调用 `Release()`
- `Release()` 前 panic

**解决：**
```go
// 使用 defer 确保归还
conn, err := pool.Get(ctx)
if err != nil {
    return err
}
defer conn.Release()  // 即使 panic 也会执行
```

### Q2: 连接池满了怎么办？

**症状：** Get() 阻塞或超时

**解决：**
1. 增加 MaxConn
2. 减少连接持有时间
3. 使用非阻塞获取：`GetNonBlocking()`
4. 检查是否有连接泄漏

### Q3: 连接不健康怎么办？

**症状：** 使用连接时返回错误

**解决：**
- 实现 `IsHealthy()` 检查连接状态
- Get() 时自动检测并重建
- 定期清理不健康连接

### Q4: 如何监控连接池？

```go
stats := pool.Stats()
fmt.Printf("总连接: %d, 活跃: %d, 空闲: %d\n",
    stats["total"], stats["active"], stats["idle"])
```

**监控指标：**
- 连接总数
- 活跃连接数
- 空闲连接数
- 等待获取的 goroutine 数
- 连接创建/关闭速率

---

## 七、性能对比

### 7.1 有/无连接池对比

| 场景 | 无连接池 | 有连接池 | 提升 |
|------|----------|----------|------|
| 单次请求延迟 | 10ms (建连) | 0.1ms (复用) | 100x |
| 1000 QPS 内存 | 500MB | 50MB | 10x |
| 连接数 | 1000+ | 10 | 100x |

### 7.2 连接池参数调优

**高并发场景：**
```go
Config{
    InitialConn: 10,      // 预创建更多
    MaxConn:     100,     // 允许更多连接
    MaxIdle:     50,      // 保持更多空闲
    MaxLifetime: 1 hour,  // 连接长期复用
}
```

**低延迟场景：**
```go
Config{
    InitialConn: 5,
    MaxConn:     20,
    MaxIdle:     10,
    ConnTimeout: 1 second, // 快速失败
}
```

---

## 八、最佳实践

1. **每个目标地址一个池**
   ```go
   pools map[string]*GRPCPool
   ```

2. **使用 defer Release()**
   ```go
   conn, _ := pool.Get(ctx)
   defer conn.Release()
   ```

3. **设置合理超时**
   ```go
   ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
   ```

4. **定期监控**
   ```go
   go func() {
       for {
           time.Sleep(30 * time.Second)
           log.Printf("Pool stats: %v", pool.Stats())
       }
   }()
   ```

5. **优雅关闭**
   ```go
   func (s *Server) Shutdown() {
       for _, p := range s.pools {
           p.Close()
       }
   }
   ```

